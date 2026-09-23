package engine

import (
	"fmt"
	"strings"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/rete"
)

// ActionContext encapsulates execution state for RHS rule actions.
// Context instances are pooled and reused across cycles to achieve zero allocation.
type ActionContext struct {
	Engine        *Engine
	Activation    *conflict.Activation
	Token         *rete.Token
	localBindings map[string]model.Value
	wmes          []*model.WME
	wmesPopulated bool
}

// Reset clears the ActionContext for reuse in the sync.Pool.
func (ctx *ActionContext) Reset() {
	ctx.Engine = nil
	ctx.Activation = nil
	ctx.Token = nil
	for k := range ctx.localBindings {
		delete(ctx.localBindings, k)
	}
	ctx.wmes = ctx.wmes[:0]
	ctx.wmesPopulated = false
}

// GetVariable returns the bound value of variable `name`, checking local action overrides first,
// then walking the token spine. If not bound, it returns the provided fallback.
func (ctx *ActionContext) GetVariable(name string, fallback model.Value) model.Value {
	if len(ctx.localBindings) > 0 {
		if val, ok := ctx.localBindings[name]; ok {
			return val
		}
		if len(name) > 2 && name[0] == '<' && name[len(name)-1] == '>' {
			if val, ok := ctx.localBindings[name[1:len(name)-1]]; ok {
				return val
			}
		} else {
			if val, ok := ctx.localBindings["<"+name+">"]; ok {
				return val
			}
		}
	}
	if ctx.Token != nil {
		if val, ok := ctx.Token.GetBinding(name); ok {
			return val
		}
		if len(name) > 2 && name[0] == '<' && name[len(name)-1] == '>' {
			if val, ok := ctx.Token.GetBinding(name[1:len(name)-1]); ok {
				return val
			}
		} else {
			if val, ok := ctx.Token.GetBinding("<" + name + ">"); ok {
				return val
			}
		}
	}
	return fallback
}

// SetVariable records a dynamic variable binding introduced on the RHS (e.g. via bind, cbind, or modify).
func (ctx *ActionContext) SetVariable(name string, val model.Value) {
	if ctx.localBindings == nil {
		ctx.localBindings = make(map[string]model.Value)
	}
	stripped := strings.TrimPrefix(strings.TrimSuffix(name, ">"), "<")
	ctx.localBindings[stripped] = val
}

// AllBindings materializes a full map of variable bindings when needed (e.g. for dynamic rule compilation in build).
func (ctx *ActionContext) AllBindings() map[string]model.Value {
	var m map[string]model.Value
	if ctx.Token != nil {
		m = ctx.Token.Bindings()
	}
	if m == nil {
		m = make(map[string]model.Value)
	}
	for k, v := range ctx.localBindings {
		m[k] = v
	}
	return m
}

// GetWMEs returns all WMEs in condition element order, cached on first access.
func (ctx *ActionContext) GetWMEs() []*model.WME {
	if !ctx.wmesPopulated {
		if ctx.Token != nil {
			ctx.wmes = append(ctx.wmes, ctx.Token.WMEs()...)
		}
		ctx.wmesPopulated = true
	}
	return ctx.wmes
}

func (e *Engine) acquireActionContext(act *conflict.Activation) *ActionContext {
	ctx := e.actionContextPool.Get().(*ActionContext)
	ctx.Engine = e
	ctx.Activation = act
	if act != nil {
		ctx.Token = act.Token
	}
	return ctx
}

func (e *Engine) releaseActionContext(ctx *ActionContext) {
	ctx.Reset()
	e.actionContextPool.Put(ctx)
}

// ValueEvaluator is a precompiled closure that produces a model.Value in the current action context.
type ValueEvaluator func(ctx *ActionContext) model.Value

// TargetResolver is a precompiled closure that resolves a target WME timetag.
type TargetResolver func(ctx *ActionContext) (int64, error)

// CompiledAction is an executable closure representing a single RHS action.
type CompiledAction func(ctx *ActionContext) error

// CompiledRule contains the precompiled RHS actions for a rule.
type CompiledRule struct {
	Name    string
	Actions []CompiledAction
}

type compiledAttr struct {
	name string
	eval ValueEvaluator
}

// compileValue transforms a model.Value expression into an optimized ValueEvaluator closure.
func (e *Engine) compileValue(val model.Value) ValueEvaluator {
	if val.IsVariable() {
		vName := val.VariableName()
		fallback := val
		return func(ctx *ActionContext) model.Value {
			return ctx.GetVariable(vName, fallback)
		}
	}
	if val.IsCompute() {
		return e.compileCompute(val.ComputeExpr())
	}
	if val.IsVector() {
		return e.compileVector(val)
	}
	if val.IsAccept() {
		ae := val.AcceptExpr()
		return func(ctx *ActionContext) model.Value {
			var res model.Value
			var err error
			if ae.IsLine {
				res, err = ctx.Engine.readAcceptLineLocked(ae.LogicalFile)
			} else {
				res, err = ctx.Engine.readAcceptLocked(ae.LogicalFile)
			}
			if err == nil {
				return res
			}
			return model.NewSymbol("nil")
		}
	}
	if val.IsGenatom() {
		return func(ctx *ActionContext) model.Value {
			return ctx.Engine.genatomLocked()
		}
	}
	if val.IsLitval() {
		le := val.LitvalExpr()
		class := le.Class
		isClassVar := strings.HasPrefix(class, "<") && strings.HasSuffix(class, ">")
		var classVarName string
		if isClassVar {
			classVarName = strings.TrimPrefix(strings.TrimSuffix(class, ">"), "<")
		}
		attrEval := e.compileValue(le.Attribute)
		return func(ctx *ActionContext) model.Value {
			cls := class
			if isClassVar {
				classVal := ctx.GetVariable(classVarName, model.NewVariable(class))
				if classVal.Type() == model.TypeSymbol || classVal.Type() == model.TypeString {
					cls = classVal.Raw().(string)
				}
			}
			attrVal := attrEval(ctx)
			attrName := attrVal.String()
			if attrVal.Type() == model.TypeSymbol || attrVal.Type() == model.TypeString {
				attrName = attrVal.Raw().(string)
			}
			if idx, ok := ctx.Engine.litvalLocked(cls, attrName); ok {
				return model.NewInt(int64(idx))
			}
			return model.NewSymbol("nil")
		}
	}
	if val.IsSubstr() {
		se := val.SubstrExpr()
		return func(ctx *ActionContext) model.Value {
			return ctx.Engine.evaluateSubstrLocked(se, ctx.AllBindings())
		}
	}

	// Constant value (Integer, Float, Symbol, String, Boolean, etc.)
	return func(ctx *ActionContext) model.Value {
		return val
	}
}

// compileCompute creates an arithmetic evaluation closure, specializing the common binary operation case.
func (e *Engine) compileCompute(expr *model.ComputeExpr) ValueEvaluator {
	if expr == nil || len(expr.Operands) == 0 {
		zero := model.NewInt(0)
		return func(ctx *ActionContext) model.Value {
			return zero
		}
	}

	operandEvals := make([]ValueEvaluator, len(expr.Operands))
	for i, op := range expr.Operands {
		operandEvals[i] = e.compileValue(op)
	}

	ops := make([]model.ComputeOp, len(expr.Operators))
	copy(ops, expr.Operators)

	// Binary optimization (very common in OPS5 rules like (compute <var> + 1))
	if len(operandEvals) == 2 && len(ops) == 1 {
		eval0 := operandEvals[0]
		eval1 := operandEvals[1]
		op := ops[0]
		return func(ctx *ActionContext) model.Value {
			v0 := eval0(ctx)
			v1 := eval1(ctx)
			res, err := model.ApplyArithmeticOp(v0, op, v1)
			if err != nil {
				return model.NewInt(0)
			}
			return res
		}
	}

	// General N-ary compute
	return func(ctx *ActionContext) model.Value {
		current := operandEvals[0](ctx)
		for i := 0; i < len(ops) && i+1 < len(operandEvals); i++ {
			next := operandEvals[i+1](ctx)
			res, err := model.ApplyArithmeticOp(current, ops[i], next)
			if err != nil {
				return model.NewInt(0)
			}
			current = res
		}
		return current
	}
}

// compileVector precompiles vector element evaluators.
func (e *Engine) compileVector(val model.Value) ValueEvaluator {
	elems := val.VectorElements()
	elemEvals := make([]ValueEvaluator, len(elems))
	for i, el := range elems {
		elemEvals[i] = e.compileValue(el)
	}
	return func(ctx *ActionContext) model.Value {
		resolved := make([]model.Value, 0, len(elemEvals))
		for _, elEval := range elemEvals {
			r := elEval(ctx)
			if r.IsVector() {
				resolved = append(resolved, r.VectorElements()...)
			} else {
				resolved = append(resolved, r)
			}
		}
		return model.NewVector(resolved)
	}
}

// compileTargetResolver constructs an optimized closure to locate the target WME timetag.
func compileTargetResolver(elemVar string, index int) TargetResolver {
	if elemVar != "" {
		vName := strings.TrimPrefix(strings.TrimSuffix(elemVar, ">"), "<")
		errElem := fmt.Errorf("element variable <%s> not bound to a valid timetag", elemVar)
		return func(ctx *ActionContext) (int64, error) {
			bound := ctx.GetVariable(vName, model.Value{})
			if bound.Type() == model.TypeInteger {
				return bound.Raw().(int64), nil
			}
			return 0, errElem
		}
	}

	return func(ctx *ActionContext) (int64, error) {
		if ctx.Token != nil {
			if wme, ok := ctx.Token.WMEAt(index); ok {
				return wme.Timetag, nil
			}
			wmes := ctx.GetWMEs()
			return 0, fmt.Errorf("invalid condition element index %d (token has %d WMEs)", index, len(wmes))
		}
		return 0, fmt.Errorf("invalid condition element index %d (token has 0 WMEs)", index)
	}
}

// compileMakeAction compiles a (make ...) action into an optimized closure.
func (e *Engine) compileMakeAction(act model.MakeAction) CompiledAction {
	class := act.Class
	orderedKeys := e.getOrderedAttributeKeys(class, act.Attributes)
	attrs := make([]compiledAttr, len(orderedKeys))
	for i, k := range orderedKeys {
		attrs[i] = compiledAttr{
			name: model.NormalizeAttribute(k),
			eval: e.compileValue(act.Attributes[k]),
		}
	}

	return func(ctx *ActionContext) error {
		resolvedAttrs := make(map[string]model.Value, len(attrs))
		for i := 0; i < len(attrs); i++ {
			resolvedAttrs[attrs[i].name] = attrs[i].eval(ctx)
		}
		wme := ctx.Engine.wm.Make(class, resolvedAttrs)
		ctx.Engine.lastAddedTimetag = wme.Timetag
		ctx.Engine.logWMAssertLocked(wme)
		return nil
	}
}

// compileModifyAction compiles a (modify ...) action into an optimized closure.
func (e *Engine) compileModifyAction(rule *model.Rule, act model.ModifyAction) CompiledAction {
	resolver := compileTargetResolver(act.TargetElementVar, act.TargetIndex)
	hasElemVar := (act.TargetElementVar != "")
	elemVarName := ""
	if hasElemVar {
		elemVarName = strings.TrimPrefix(strings.TrimSuffix(act.TargetElementVar, ">"), "<")
	}

	var targetClass string
	if rule != nil {
		if act.TargetIndex >= 1 && act.TargetIndex <= len(rule.Conditions) {
			targetClass = rule.Conditions[act.TargetIndex-1].Class
		} else if act.TargetElementVar != "" {
			targetNorm := strings.TrimPrefix(strings.TrimSuffix(act.TargetElementVar, ">"), "<")
			for _, ce := range rule.Conditions {
				ceVarNorm := strings.TrimPrefix(strings.TrimSuffix(ce.ElementVariable, ">"), "<")
				if ceVarNorm == targetNorm {
					targetClass = ce.Class
					break
				}
			}
		}
	}

	orderedKeys := e.getOrderedAttributeKeys(targetClass, act.Attributes)
	attrs := make([]compiledAttr, len(orderedKeys))
	for i, k := range orderedKeys {
		attrs[i] = compiledAttr{
			name: model.NormalizeAttribute(k),
			eval: e.compileValue(act.Attributes[k]),
		}
	}

	return func(ctx *ActionContext) error {
		targetTimetag, err := resolver(ctx)
		if err != nil {
			return err
		}

		var oldWme *model.WME
		if ctx.Engine.watchLevel >= 2 {
			oldWme, _ = ctx.Engine.wm.Get(targetTimetag)
		}

		resolvedAttrs := make(map[string]model.Value, len(attrs))
		for i := 0; i < len(attrs); i++ {
			resolvedAttrs[attrs[i].name] = attrs[i].eval(ctx)
		}

		newWme, err := ctx.Engine.wm.Modify(targetTimetag, resolvedAttrs)
		if err != nil {
			return err
		}

		ctx.Engine.lastAddedTimetag = newWme.Timetag
		if hasElemVar {
			ctx.SetVariable(elemVarName, model.NewInt(newWme.Timetag))
		}
		if oldWme != nil {
			ctx.Engine.logWMRetractLocked(oldWme)
		}
		ctx.Engine.logWMAssertLocked(newWme)
		return nil
	}
}

// compileRemoveAction compiles a (remove ...) action into an optimized closure.
func compileRemoveAction(act model.RemoveAction) CompiledAction {
	if act.Wildcard {
		return func(ctx *ActionContext) error {
			removed := ctx.Engine.wm.RemoveAll()
			ctx.Engine.lastAddedTimetag = 0
			for _, w := range removed {
				ctx.Engine.logWMRetractLocked(w)
			}
			return nil
		}
	}

	resolver := compileTargetResolver(act.TargetElementVar, act.TargetIndex)
	return func(ctx *ActionContext) error {
		targetTimetag, err := resolver(ctx)
		if err != nil {
			return err
		}
		oldWme, err := ctx.Engine.wm.Remove(targetTimetag)
		if err != nil {
			return err
		}
		if oldWme != nil {
			ctx.Engine.logWMRetractLocked(oldWme)
		}
		return nil
	}
}

// compileWriteAction compiles a (write ...) action into an optimized closure.
func (e *Engine) compileWriteAction(act model.WriteAction) CompiledAction {
	type compiledWriteArg struct {
		argType model.WriteArgType
		eval    ValueEvaluator
	}

	hasCRLF := false
	for _, arg := range act.Args {
		if arg.Type == model.WriteArgCRLF {
			hasCRLF = true
			break
		}
	}

	compiledArgs := make([]compiledWriteArg, len(act.Args))
	for i, arg := range act.Args {
		compiledArgs[i].argType = arg.Type
		if arg.Type != model.WriteArgCRLF {
			compiledArgs[i].eval = e.compileValue(arg.Value)
		}
	}

	return func(ctx *ActionContext) error {
		ww := ctx.Engine.outputWriter
		if ctx.Engine.defaultWriteStream != "" {
			if entry, ok := ctx.Engine.openFiles[ctx.Engine.defaultWriteStream]; ok && entry.writer != nil {
				ww = entry.writer
			}
		}
		if ww == nil {
			return nil
		}

		lastWasSpaceOrTab := (ctx.Engine.currentCol == 1)
		for _, arg := range compiledArgs {
			switch arg.argType {
			case model.WriteArgCRLF:
				fmt.Fprint(ww, "\n")
				ctx.Engine.currentCol = 1
				lastWasSpaceOrTab = true

			case model.WriteArgTabTo:
				targetCol := 1
				resolvedCol := arg.eval(ctx)
				if resolvedCol.Type() == model.TypeInteger {
					targetCol = int(resolvedCol.Raw().(int64))
				} else if resolvedCol.Type() == model.TypeFloat {
					targetCol = int(resolvedCol.Raw().(float64))
				}
				if targetCol < 1 {
					targetCol = 1
				}

				if targetCol > ctx.Engine.currentCol {
					spaces := strings.Repeat(" ", targetCol-ctx.Engine.currentCol)
					fmt.Fprint(ww, spaces)
					ctx.Engine.currentCol = targetCol
				} else {
					fmt.Fprint(ww, " ")
					ctx.Engine.currentCol++
				}
				lastWasSpaceOrTab = true

			case model.WriteArgValue:
				resolved := arg.eval(ctx)
				var text string
				if resolved.Type() == model.TypeString {
					text = resolved.Raw().(string)
				} else {
					text = resolved.String()
				}

				if !lastWasSpaceOrTab {
					fmt.Fprint(ww, " ")
					ctx.Engine.currentCol++
				}

				fmt.Fprint(ww, text)
				for _, r := range text {
					if r == '\n' {
						ctx.Engine.currentCol = 1
					} else {
						ctx.Engine.currentCol++
					}
				}
				lastWasSpaceOrTab = false
			}
		}

		if !hasCRLF {
			fmt.Fprint(ww, "\n")
			ctx.Engine.currentCol = 1
		}
		return nil
	}
}

// compileOpenFileAction compiles an (openfile ...) action.
func (e *Engine) compileOpenFileAction(act model.OpenFileAction) CompiledAction {
	filespecEval := e.compileValue(act.Filespec)
	mode := act.Mode
	logicalName := act.LogicalName

	return func(ctx *ActionContext) error {
		resolved := filespecEval(ctx)
		var filespec string
		if resolved.Type() == model.TypeString {
			filespec = resolved.Raw().(string)
		} else {
			filespec = resolved.String()
		}
		return ctx.Engine.openFileLocked(logicalName, filespec, mode)
	}
}

// compileCloseFileAction compiles a (closefile ...) action.
func (e *Engine) compileCloseFileAction(act model.CloseFileAction) CompiledAction {
	logicalName := act.LogicalName
	return func(ctx *ActionContext) error {
		return ctx.Engine.closeFileLocked(logicalName)
	}
}

// compileDefaultAction compiles a (default ...) action.
func (e *Engine) compileDefaultAction(act model.DefaultAction) CompiledAction {
	logicalName := act.LogicalName
	subsystem := act.Subsystem
	return func(ctx *ActionContext) error {
		return ctx.Engine.setDefaultLocked(logicalName, subsystem)
	}
}

// compileBindAction compiles a (bind ...) action.
func (e *Engine) compileBindAction(act model.BindAction) CompiledAction {
	varName := strings.TrimPrefix(strings.TrimSuffix(act.Variable, ">"), "<")
	valEval := e.compileValue(act.Value)
	return func(ctx *ActionContext) error {
		resolved := valEval(ctx)
		ctx.SetVariable(varName, resolved)
		return nil
	}
}

// compileCBindAction compiles a (cbind ...) action.
func (e *Engine) compileCBindAction(act model.CBindAction) CompiledAction {
	varName := strings.TrimPrefix(strings.TrimSuffix(act.Variable, ">"), "<")
	return func(ctx *ActionContext) error {
		if ctx.Engine.lastAddedTimetag == 0 {
			return fmt.Errorf("cbind: no element has been added to working memory")
		}
		ctx.SetVariable(varName, model.NewInt(ctx.Engine.lastAddedTimetag))
		return nil
	}
}

// compileWatchAction compiles a (watch ...) action.
func (e *Engine) compileWatchAction(act model.WatchAction) CompiledAction {
	level := act.Level
	return func(ctx *ActionContext) error {
		if level == nil {
			tw := ctx.Engine.traceWriterLocked()
			if tw != nil {
				fmt.Fprintf(tw, "Current watch level: %d\n", ctx.Engine.watchLevel)
			}
		} else {
			ctx.Engine.watchLevel = *level
			ctx.Engine.traceEnabled = (*level > 0)
		}
		return nil
	}
}

// compileHaltAction compiles a (halt) action.
func (e *Engine) compileHaltAction() CompiledAction {
	return func(ctx *ActionContext) error {
		ctx.Engine.halted = true
		return nil
	}
}

// compileCustomAction compiles a custom extension action.
func (e *Engine) compileCustomAction(act model.CustomAction) CompiledAction {
	return func(ctx *ActionContext) error {
		if act.Execute != nil {
			return act.Execute(ctx.Activation)
		}
		return nil
	}
}

// compileBuildAction compiles a dynamic (build ...) action.
func (e *Engine) compileBuildAction(act model.BuildAction) CompiledAction {
	return func(ctx *ActionContext) error {
		if act.Rule != nil {
			newRule := substituteRuleBindings(act.Rule, ctx.AllBindings())
			ctx.Engine.addRuleLocked(newRule)
			if ctx.Engine.traceEnabled && ctx.Engine.watchLevel >= 1 {
				tw := ctx.Engine.traceWriterLocked()
				if tw != nil {
					fmt.Fprintf(tw, "==> Built and compiled rule '%s'\n", newRule.Name)
				}
			}
		}
		return nil
	}
}

// compileRule precompiles all RHS actions of a rule into callable closures.
func (e *Engine) compileRule(rule *model.Rule) *CompiledRule {
	if rule == nil {
		return nil
	}

	actions := make([]CompiledAction, 0, len(rule.Actions))
	for _, a := range rule.Actions {
		switch act := a.(type) {
		case model.WatchAction:
			actions = append(actions, e.compileWatchAction(act))
		case model.BindAction:
			actions = append(actions, e.compileBindAction(act))
		case model.CBindAction:
			actions = append(actions, e.compileCBindAction(act))
		case model.MakeAction:
			actions = append(actions, e.compileMakeAction(act))
		case model.ModifyAction:
			actions = append(actions, e.compileModifyAction(rule, act))
		case model.RemoveAction:
			actions = append(actions, compileRemoveAction(act))
		case model.OpenFileAction:
			actions = append(actions, e.compileOpenFileAction(act))
		case model.CloseFileAction:
			actions = append(actions, e.compileCloseFileAction(act))
		case model.DefaultAction:
			actions = append(actions, e.compileDefaultAction(act))
		case model.WriteAction:
			actions = append(actions, e.compileWriteAction(act))
		case model.HaltAction:
			actions = append(actions, e.compileHaltAction())
		case model.CustomAction:
			actions = append(actions, e.compileCustomAction(act))
		case model.BuildAction:
			actions = append(actions, e.compileBuildAction(act))
		}
	}

	return &CompiledRule{
		Name:    rule.Name,
		Actions: actions,
	}
}

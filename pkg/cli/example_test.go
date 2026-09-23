package cli_test

import (
	"fmt"

	"github.com/graemenewlands/ops5/pkg/cli"
)

func ExampleTable() {
	table := cli.NewTable(nil)
	table.Unicode = false // ASCII borders for deterministic test output
	table.SetHeaders("Timetag", "Class", "Attributes")
	table.AddRow("1", "order", "^id 101 ^status pending")
	table.AddRow("2", "order", "^id 102 ^status completed")

	fmt.Print(table.Render())

	// Output:
	// +---------+-------+---------------------------+
	// | Timetag | Class | Attributes                |
	// +---------+-------+---------------------------+
	// | 1       | order | ^id 101 ^status pending   |
	// | 2       | order | ^id 102 ^status completed |
	// +---------+-------+---------------------------+
}

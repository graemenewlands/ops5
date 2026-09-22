package manners

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
)

const mannersRules = `;;; Miss Manners Benchmark Problem
;;; Reference: https://github.com/johanlindberg/ops5

(literalize guest name sex hobby)
(literalize last_seat seat)
(literalize seating seat1 name1 name2 seat2 id pid path_done)
(literalize context state)
(literalize path id name seat)
(literalize chosen id name hobby)
(literalize count c)

(p assign_first_seat
   (context ^state start)
   (guest ^name <n>)
   (count ^c <c>)
-->
   (make seating ^seat1 1 ^name1 <n> ^name2 <n> ^seat2 1 ^id <c> ^pid 0 ^path_done yes)
   (make path ^id <c> ^name <n> ^seat 1)
   (modify 3 ^c (compute <c> + 1))
   (modify 1 ^state assign_seats)
)

(p find_seating
   (context ^state assign_seats)
   (seating ^seat1 <s1> ^name1 <n1> ^name2 <n2> ^seat2 <s2> ^id <id> ^pid <pid> ^path_done yes)
   (guest ^name <n2> ^sex <s> ^hobby <h>)
   (guest ^name <g_new> ^sex <> <s> ^hobby <h>)
   (count ^c <c>)
   -(path ^id <id> ^name <g_new>)
   -(chosen ^id <id> ^name <g_new> ^hobby <h>)
-->
   (make seating ^seat1 <s2> ^name1 <n2> ^name2 <g_new> ^seat2 (compute <s2> + 1) ^id <c> ^pid <id> ^path_done no)
   (make path ^id <c> ^name <g_new> ^seat (compute <s2> + 1))
   (make chosen ^id <id> ^name <g_new> ^hobby <h>)
   (modify 5 ^c (compute <c> + 1))
   (modify 1 ^state make_path)
)

(p make_path
   (context ^state make_path)
   (seating ^id <id> ^pid <pid> ^path_done no)
   (path ^id <pid> ^name <n> ^seat <s>)
   -(path ^id <id> ^name <n>)
-->
   (make path ^id <id> ^name <n> ^seat <s>)
)

(p path_done
   (context ^state make_path)
   {(seating ^path_done no) <seat>}
-->
   (modify <seat> ^path_done yes)
   (modify 1 ^state check_done)
)

(p are_we_done
   (context ^state check_done)
   (last_seat ^seat <s_last>)
   (seating ^seat2 <s_last> ^name2 <last_guest> ^id <id>)
   (seating ^pid 0 ^name1 <first_guest>)
   (guest ^name <last_guest> ^sex <sex_last> ^hobby <h>)
   (guest ^name <first_guest> ^sex <> <sex_last> ^hobby <h>)
-->
   (write "Found solution for " <s_last> " guests." (crlf))
   (modify 1 ^state print_results)
)

(p continue
   (context ^state check_done)
-->
   (modify 1 ^state assign_seats)
)

(p print_results
   (context ^state print_results)
   (last_seat ^seat <s_last>)
   (seating ^seat2 <s_last> ^id <id>)
   (path ^id <id> ^name <n> ^seat <s>)
-->
   (write "Seat " <s> ": Guest " <n> (crlf))
)

(p all_done
   (context ^state print_results)
-->
   (halt)
)
`

// GenerateMannersOps generates an OPS5 source string for Miss Manners with N guests.
func GenerateMannersOps(numGuests int, seed int64) string {
	var sb strings.Builder
	sb.WriteString(mannersRules)
	sb.WriteString("\n")

	sb.WriteString("(make context ^state start)\n")
	sb.WriteString("(make count ^c 1)\n")
	sb.WriteString(fmt.Sprintf("(make last_seat ^seat %d)\n\n", numGuests))

	r := rand.New(rand.NewSource(seed))
	numHobbies := 8

	// cycleHobbies[i] connects guest i and guest (i % numGuests) + 1
	cycleHobbies := make([]int, numGuests)
	for i := 0; i < numGuests; i++ {
		cycleHobbies[i] = (i % numHobbies) + 1
	}

	for i := 1; i <= numGuests; i++ {
		sex := "m"
		if i%2 == 0 {
			sex = "f"
		}

		hobbies := make(map[int]bool)
		prevHobby := cycleHobbies[(i-2+numGuests)%numGuests]
		nextHobby := cycleHobbies[i-1]
		hobbies[prevHobby] = true
		hobbies[nextHobby] = true

		if r.Float64() < 0.25 {
			extra := r.Intn(numHobbies) + 1
			hobbies[extra] = true
		}

		var sortedHobbies []int
		for h := range hobbies {
			sortedHobbies = append(sortedHobbies, h)
		}
		sort.Ints(sortedHobbies)

		for _, h := range sortedHobbies {
			sb.WriteString(fmt.Sprintf("(make guest ^name %d ^sex %s ^hobby h%d)\n", i, sex, h))
		}
	}

	return sb.String()
}

// WriteMannersFiles generates manners16.ops, manners32.ops, manners64.ops, manners128.ops if not already present.
func WriteMannersFiles(dir string) error {
	sizes := []int{16, 32, 64, 128}
	for _, n := range sizes {
		path := fmt.Sprintf("%s/manners%d.ops", dir, n)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		opsContent := GenerateMannersOps(n, 42)
		if err := os.WriteFile(path, []byte(opsContent), 0644); err != nil {
			return err
		}
	}
	return nil
}

package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: forcejob <campaign>")
		fmt.Fprintln(os.Stderr, "campaigns: birthday, anniversary, leftover-cart")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "birthday":
		runBirthday()
	case "anniversary":
		runAnniversary()
	case "leftover-cart":
		runLeftoverCart()
	default:
		log.Fatalf("unknown campaign: %q  (valid: birthday, anniversary, leftover-cart)", os.Args[1])
	}
}

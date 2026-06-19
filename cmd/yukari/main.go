package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: yukari <campaign>")
		fmt.Fprintln(os.Stderr, "campaigns: birthday, anniversary, leftover-cart, discounted-wishlist, wishlist-back-in, winback")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "birthday":
		runBirthday()
	case "anniversary":
		runAnniversary()
	case "leftover-cart":
		runLeftoverCart()
	case "discounted-wishlist":
		runDiscountedWishlist()
	case "winback":
		runWinback()
	case "wishlist-back-in":
		runWishlistBackIn()
	default:
		log.Fatalf("unknown campaign: %q  (valid: birthday, anniversary, leftover-cart, discounted-wishlist, wishlist-back-in, winback)", os.Args[1])
	}
}

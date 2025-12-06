package main

import (
	"fmt"
	"log"

	"github.com/tompston/syro"
)

func main() {
	res, err := syro.NewRequest("GET", "https://httpbin.org/get").Do()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%v\n", res.Summary())
}

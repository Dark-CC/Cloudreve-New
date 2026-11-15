// Command hashidtool prints Cloudreve hash IDs for local verification.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: hashidtool <salt> <type> <id>")
		os.Exit(2)
	}

	salt := os.Args[1]
	idType, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	id, err := strconv.Atoi(os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	encoder, err := hashid.New(salt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch idType {
	case hashid.ShareID:
		fmt.Println(hashid.EncodeShareID(encoder, id))
	case hashid.UserID:
		fmt.Println(hashid.EncodeUserID(encoder, id))
	default:
		out, err := encoder.Encode([]int{id, idType})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(out)
	}
}

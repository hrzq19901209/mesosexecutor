package main

import (
	"fmt"
	"github.com/docker/engine-api/types/strslice"
)

func main() {
	var cmd strslice.StrSlice = []string{"--port=1314"}
	fmt.Println(cmd)
}

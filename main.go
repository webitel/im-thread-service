package main

import (
	"fmt"

	"github.com/webitel/im-thread-service/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		fmt.Println(err.Error())
		return
	}
}

package main

import (
	"fmt"
	"os"
)

func checkErr(err error) {
	if err == nil {
		return
	}
	fmt.Println("Error:", err)
	os.Exit(1)
}

func main() {
	serial := "D6DC8DB0"
	ep := 0x85
	list, err := FindAll(0x1d50, 0x6018)
	checkErr(err)
	var h *USBDH
	for _, d := range list {
		if d.Serial == serial {
			h, err = d.Open()
			checkErr(err)
			break
		}
	}
	if h == nil {
		fmt.Println("not found")
		return
	}
	buf := make([]byte, 256)
	for {
		n, err := h.Read(ep, buf)
		checkErr(err)
		fmt.Println(buf[:n])
	}
}

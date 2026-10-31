//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// pauseOnError keeps an Explorer-launched console open so an error stays
// visible. It only waits when hax is the only process attached to the
// console, which means Explorer started the program with a new window.
func pauseOnError() {
	if !ownsConsole() {
		return
	}
	_, _ = fmt.Fprintln(os.Stderr, "\nPress Enter to close...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func ownsConsole() bool {
	var pids [2]uint32
	count, _, _ := getConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&pids[0])),
		uintptr(len(pids)),
	)
	return count == 1
}

//go:build !windows

package main

// pauseOnError does nothing outside Windows, where the terminal stays open.
func pauseOnError() {}

//go:build !windows

package main

func raiseGadakWindowByTitle(string) bool { return false }

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "performance":
		performanceCommand()
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf("Usage: %s <command>\n", os.Args[0])
	fmt.Printf("\nCommands:\n")
	fmt.Printf("  performance  Run SSZ performance benchmarks\n")
	fmt.Printf("  help         Show this help message\n")
}

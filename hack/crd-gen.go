//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Custom CRD generation that includes post-processing
func main() {
	// Run controller-gen
	cmd := exec.Command("controller-gen",
		"rbac:roleName=manager-role",
		"crd",
		"webhook",
		"paths=./...",
		"output:crd:artifacts:config=config/crd/bases")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running controller-gen: %v\n", err)
		os.Exit(1)
	}

	// Apply CRD fixes
	fixCmd := exec.Command("python3", "hack/fix-recursive-crd.py")
	fixCmd.Stdout = os.Stdout
	fixCmd.Stderr = os.Stderr

	if err := fixCmd.Run(); err != nil {
		fmt.Printf("Error applying CRD fixes: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ CRD generation with fixes completed successfully!")
}

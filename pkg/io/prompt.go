// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package io

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func ContinuePrompt(prompt, declinedMsg string) error {
	for {
		reader := bufio.NewReader(os.Stdin)

		fmt.Printf("%s (Y/N): ", prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("Error reading input: %w", err)
		}

		input = strings.ToUpper(strings.TrimSpace(input))

		if input == "Y" {
			return nil
		} else if input == "N" {
			return fmt.Errorf(declinedMsg)
		} else {
			fmt.Println("Invalid input. Please enter 'Y' or 'N'.")
		}
	}
}

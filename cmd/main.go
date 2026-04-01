package main

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"github.com/nick/profuse/internal/auth"
	"github.com/nick/profuse/internal/fs"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func main() {
	root := &cobra.Command{
		Use:   "profuse",
		Short: "Mount Proton Drive as a local filesystem",
	}

	root.AddCommand(cmdAuth(), cmdMount())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func cmdAuth() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}
	cmd.AddCommand(cmdAuthLogin(), cmdAuthLogout(), cmdAuthStatus())
	return cmd
}

func cmdAuthLogin() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Proton and store credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print("Username: ")
			var username string
			fmt.Scanln(&username)

			password, err := readPassword("Password: ")
			if err != nil {
				return err
			}

			get2FACode := func() string {
				fmt.Print("2FA code: ")
				var code string
				fmt.Scanln(&code)
				return code
			}

			ctx := context.Background()
			sess, err := auth.Login(ctx, username, password, get2FACode)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			if err := sess.Save(); err != nil {
				return fmt.Errorf("saving session: %w", err)
			}

			fmt.Println("Logged in. Key password stored in OS keyring.")
			return nil
		},
	}
}

func cmdAuthLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke session and remove stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := auth.LoadSession()
			if err != nil {
				return fmt.Errorf("no active session: %w", err)
			}
			ctx := context.Background()
			if err := sess.Logout(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "warn: server logout failed: %v\n", err)
			}
			if err := auth.DeleteSession(); err != nil {
				return err
			}
			fmt.Println("Logged out.")
			return nil
		},
	}
}

func cmdAuthStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := auth.LoadSession()
			if err != nil {
				fmt.Println("Not logged in.")
				return nil
			}
			fmt.Printf("Logged in as: %s\n", sess.Username)
			return nil
		},
	}
}

func cmdMount() *cobra.Command {
	var debug bool

	cmd := &cobra.Command{
		Use:   "mount <mountpoint>",
		Short: "Mount Proton Drive (reads key password from OS keyring)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mountpoint := args[0]

			if info, err := os.Stat(mountpoint); err != nil || !info.IsDir() {
				return fmt.Errorf("mountpoint %q must be an existing directory", mountpoint)
			}

			sess, err := auth.LoadSession()
			if err != nil {
				return err
			}

			ctx := context.Background()
			client, addrKR, err := sess.Unlock(ctx)
			if err != nil {
				return err
			}

			fmt.Printf("Mounting Proton Drive at %s\n", mountpoint)
			return fs.Mount(ctx, mountpoint, client, addrKR, fs.Options{
				Debug:    debug,
				ReadOnly: true,
			})
		},
	}

	cmd.Flags().BoolVar(&debug, "debug", false, "Enable FUSE debug logging")
	return cmd
}

func readPassword(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	return password, err
}

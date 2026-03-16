package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"equinox/auth"
	"equinox/store"

	"github.com/spf13/cobra"
)

var (
	userEmail    string
	userPassword string
	userRole     string
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage dashboard users",
}

var userAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new dashboard user",
	RunE:  runUserAdd,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all dashboard users",
	RunE:  runUserList,
}

func init() {
	userAddCmd.Flags().StringVar(&userEmail, "email", "", "User email address (required)")
	userAddCmd.Flags().StringVar(&userPassword, "password", "", "User password (required)")
	userAddCmd.Flags().StringVar(&userRole, "role", "viewer", "User role")
	userAddCmd.MarkFlagRequired("email")
	userAddCmd.MarkFlagRequired("password")

	userCmd.AddCommand(userAddCmd)
	userCmd.AddCommand(userListCmd)
}

func runUserAdd(cmd *cobra.Command, args []string) error {
	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	a := auth.New(db)
	user, err := a.CreateUser(userEmail, userPassword, userRole)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("Created user: ID=%s Email=%s Role=%s\n", user.ID, user.Email, user.Role)
	return nil
}

func runUserList(cmd *cobra.Command, args []string) error {
	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	a := auth.New(db)
	users, err := a.ListUsers()
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEMAIL\tROLE\tCREATED")
	for _, u := range users {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.ID, u.Email, u.Role, u.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	w.Flush()

	return nil
}

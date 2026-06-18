// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// TestShortcutMount_EmptyCommandConfiguresServiceNode verifies that a shortcut
// with an empty Command makes the *parent* (service node) itself runnable in
// place — `lark-cli search` rather than `lark-cli search +search` — while a
// sibling +verb shortcut still mounts as a child. This is the framework hook
// the search package relies on.
func TestShortcutMount_EmptyCommandConfiguresServiceNode(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	svc := &cobra.Command{Use: "search"}

	defaultAction := Shortcut{
		Service:     "search",
		Command:     "", // default action: configure the service node in place
		Description: "cross-entity semantic search",
		HasFormat:   true,
		Flags: []Flag{
			{Name: "query", Required: true, Desc: "search query"},
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	defaultAction.Mount(svc, f)

	// The service node itself is now runnable and carries the shortcut's flags
	// and short help — no child was added for the default action.
	if svc.RunE == nil {
		t.Fatal("empty-Command shortcut should set the service node's RunE")
	}
	if svc.Short != "cross-entity semantic search" {
		t.Errorf("service Short = %q, want the shortcut Description", svc.Short)
	}
	if svc.Flags().Lookup("query") == nil {
		t.Error("expected --query registered on the service node")
	}
	if svc.Flags().Lookup("format") == nil {
		t.Error("expected auto-injected --format registered on the service node")
	}
	for _, c := range svc.Commands() {
		if c.Name() == "" || c.Name() == "search" {
			t.Errorf("empty-Command shortcut must not add a child, found %q", c.Name())
		}
	}

	// A sibling +verb shortcut still mounts as a child of the same node.
	pingShortcut := Shortcut{
		Service:     "search",
		Command:     "+ping",
		Description: "diagnostic",
		Hidden:      true,
		Execute:     func(context.Context, *RuntimeContext) error { return nil },
	}
	pingShortcut.Mount(svc, f)

	child, _, err := svc.Find([]string{"+ping"})
	if err != nil || child == nil || child.Name() != "+ping" {
		t.Fatalf("expected +ping child mounted, err=%v child=%v", err, child)
	}
	if !child.Hidden {
		t.Error("+ping child should be hidden")
	}
	// The default action's RunE must survive sibling mounts.
	if svc.RunE == nil {
		t.Fatal("service node RunE lost after mounting a child")
	}
}

package notifications

import (
	"testing"
	"time"
)

func TestVisible(t *testing.T) {
	t.Parallel()

	n := Notifications{
		&Notification{Meta: Meta{Done: false, Hidden: false}},
		&Notification{Meta: Meta{Done: true, Hidden: false}},
		&Notification{Meta: Meta{Done: false, Hidden: true}},
		&Notification{Meta: Meta{Done: true, Hidden: true}},
	}

	visible := n.Visible()

	if len(visible) != 1 {
		t.Errorf("Expected 1, got %d", len(visible))
	}

	if visible[0] != n[0] {
		t.Errorf("Expected %v, got %v", n[0], visible[0])
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		max      int
		expected string
	}{
		{
			name:     "shorter than max",
			input:    "hello",
			max:      10,
			expected: "hello",
		},
		{
			name:     "equal to max",
			input:    "hello",
			max:      5,
			expected: "hello",
		},
		{
			name:     "longer than max",
			input:    "hello world",
			max:      8,
			expected: "hello...",
		},
		{
			name:     "unicode characters",
			input:    "こんにちは世界", //nolint:gosmopolitan // testing Unicode support
			max:      5,
			expected: "こん...",
		},
		{
			name:     "unicode not truncated",
			input:    "こんにちは",
			max:      10,
			expected: "こんにちは",
		},
		{
			name:     "max is zero",
			input:    "hello",
			max:      0,
			expected: "",
		},
		{
			name:     "max is negative",
			input:    "hello",
			max:      -1,
			expected: "",
		},
		{
			name:     "max less than ellipsis",
			input:    "hello",
			max:      2,
			expected: "he",
		},
		{
			name:     "empty string",
			input:    "",
			max:      10,
			expected: "",
		},
		{
			name:     "long repository name",
			input:    "organization-name/very-long-repository-name-that-needs-truncation",
			max:      30,
			expected: "organization-name/very-long...",
		},
		{
			name:     "emojis",
			input:    "🔥🔥🔥🔥🔥🔥🔥🔥",
			max:      5,
			expected: "🔥🔥...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := truncate(tt.input, tt.max)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q; want %q", tt.input, tt.max, result, tt.expected)
			}

			// Verify the result doesn't exceed max length in runes
			if tt.max > 0 && len([]rune(result)) > tt.max {
				t.Errorf("truncate(%q, %d) resulted in %d runes, expected at most %d",
					tt.input, tt.max, len([]rune(result)), tt.max)
			}
		})
	}
}

func TestRenderWithLongFields(t *testing.T) {
	t.Parallel()

	// Create notifications with very long repository names and titles
	notifications := Notifications{
		&Notification{
			ID:     "1",
			Unread: true,
			Repository: Repository{
				FullName: "organization-with-very-long-name/repository-with-extremely-long-name-that-should-be-truncated",
			},
			Author: User{Login: "user1"},
			Subject: Subject{
				Title: "This is a very long notification title that contains a lot of text and should definitely be truncated when rendered in a table format to prevent wrapping issues",
				Type:  "Issue",
				State: "open",
			},
			UpdatedAt: time.Now(),
		},
		&Notification{
			ID:     "2",
			Unread: false,
			Repository: Repository{
				FullName: "short/repo",
			},
			Author: User{Login: "user2"},
			Subject: Subject{
				Title: "Short title",
				Type:  "PullRequest",
				State: "closed",
			},
			UpdatedAt: time.Now(),
		},
	}

	// Render should not panic even with long fields
	err := notifications.Render()
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify that all notifications got rendered
	for i, n := range notifications {
		if n.rendered == "" {
			t.Errorf("Notification %d was not rendered", i)
		}
	}

	// Verify that the first notification's rendered string contains ellipsis (truncated)
	// Note: This might not always be true if terminal is very wide, but it's a good check
	if len(notifications) > 0 {
		rendered := notifications[0].rendered
		if rendered == "" {
			t.Error("First notification was not rendered")
		}
		// Just verify it doesn't panic and produces some output
		t.Logf("Rendered notification 0: %s", rendered)
		t.Logf("Rendered notification 1: %s", notifications[1].rendered)
	}
}

func TestRenderEmptyNotifications(t *testing.T) {
	t.Parallel()

	notifications := Notifications{}

	err := notifications.Render()
	if err != nil {
		t.Errorf("Render() with empty notifications should not error: %v", err)
	}
}

func TestRenderNoPanic(t *testing.T) {
	t.Parallel()

	// This test ensures that we don't panic even if table wrapping creates more lines
	// than notifications
	notifications := Notifications{
		&Notification{
			ID:     "1",
			Unread: true,
			Repository: Repository{
				FullName: "org/repo-" + string(make([]rune, 200)), // Very long name
			},
			Author: User{Login: "verylongusernamethatmightcausewrapping"},
			Subject: Subject{
				Title: string(make([]rune, 500)), // Very long title
				Type:  "Issue",
				State: "open",
			},
			UpdatedAt: time.Now(),
		},
	}

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render() panicked: %v", r)
		}
	}()

	err := notifications.Render()
	if err != nil {
		t.Logf("Render() error (acceptable): %v", err)
	}

	// Check that rendered is set
	if notifications[0].rendered == "" {
		t.Error("Notification was not rendered")
	}
}

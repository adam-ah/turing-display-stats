//go:build windows

package main

import "testing"

func TestTrayMenuIDs(t *testing.T) {
	ids := map[uint32]string{
		MENU_ABOUT:       "About",
		MENU_OPEN_CONFIG: "Open Config",
		MENU_EXIT:        "Exit",
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 unique menu IDs, got %d", len(ids))
	}
	for id := range ids {
		if id == 0 {
			t.Fatal("menu ID must not be 0")
		}
	}
}

func TestTrayConstants(t *testing.T) {
	tests := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"WM_APP", WM_APP, 0x8000},
		{"WM_USER_TRAY", WM_USER_TRAY, 0x8001},
		{"WM_RBUTTONUP", WM_RBUTTONUP, 0x0205},
		{"WM_LBUTTONUP", WM_LBUTTONUP, 0x0202},
		{"WM_RBUTTONDOWN", WM_RBUTTONDOWN, 0x0204},
		{"WM_LBUTTONDOWN", WM_LBUTTONDOWN, 0x0201},
		{"WM_QUIT", WM_QUIT, 0x0012},
		{"WM_COMMAND", WM_COMMAND, 0x0111},
		{"NIM_ADD", NIM_ADD, 0x00000000},
		{"NIM_DELETE", NIM_DELETE, 0x00000002},
		{"TPM_LEFTALIGN", TPM_LEFTALIGN, 0x0000},
		{"TPM_BOTTOMALIGN", TPM_BOTTOMALIGN, 0x0020},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = 0x%X, want 0x%X", tc.name, tc.got, tc.want)
			}
		})
	}
}

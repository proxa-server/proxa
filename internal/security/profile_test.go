package security

import "testing"

func TestDefault(t *testing.T) {
	got := Default()

	if len(got.CapDrop) != 1 || got.CapDrop[0] != "ALL" {
		t.Errorf("Default().CapDrop = %v, want [ALL]", got.CapDrop)
	}
	if got.NoNewPrivileges == nil {
		t.Fatal("Default().NoNewPrivileges is nil, want pointer to true")
	}
	if !*got.NoNewPrivileges {
		t.Errorf("Default().NoNewPrivileges = false, want true")
	}
	if got.AllowRoot {
		t.Errorf("Default().AllowRoot = true, want false")
	}
	if got.User != "" {
		t.Errorf("Default().User = %q, want empty", got.User)
	}
}

func TestApply(t *testing.T) {
	tr, fa := true, false

	tests := []struct {
		name           string
		in             SecurityProfile
		wantCapDrop    []string
		wantNoNewPriv  *bool
		wantUser       string
	}{
		{
			name:          "zero value gets defaults",
			in:            SecurityProfile{},
			wantCapDrop:   []string{"ALL"},
			wantNoNewPriv: &tr,
			wantUser:      "",
		},
		{
			name:          "explicit CapDrop preserved",
			in:            SecurityProfile{CapDrop: []string{"NET_RAW"}},
			wantCapDrop:   []string{"NET_RAW"},
			wantNoNewPriv: &tr,
			wantUser:      "",
		},
		{
			name:          "explicit NoNewPrivileges=false preserved",
			in:            SecurityProfile{NoNewPrivileges: &fa, AllowRoot: true},
			wantCapDrop:   []string{"ALL"},
			wantNoNewPriv: &fa,
			wantUser:      "",
		},
		{
			name:          "explicit User preserved",
			in:            SecurityProfile{User: "1000:1000"},
			wantCapDrop:   []string{"ALL"},
			wantNoNewPriv: &tr,
			wantUser:      "1000:1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.in)

			if len(got.CapDrop) != len(tt.wantCapDrop) {
				t.Fatalf("CapDrop = %v, want %v", got.CapDrop, tt.wantCapDrop)
			}
			for i, c := range tt.wantCapDrop {
				if got.CapDrop[i] != c {
					t.Errorf("CapDrop[%d] = %q, want %q", i, got.CapDrop[i], c)
				}
			}

			if (got.NoNewPrivileges == nil) != (tt.wantNoNewPriv == nil) {
				t.Errorf("NoNewPrivileges nil mismatch: got %v, want %v", got.NoNewPrivileges, tt.wantNoNewPriv)
			} else if got.NoNewPrivileges != nil && *got.NoNewPrivileges != *tt.wantNoNewPriv {
				t.Errorf("NoNewPrivileges = %v, want %v", *got.NoNewPrivileges, *tt.wantNoNewPriv)
			}

			if got.User != tt.wantUser {
				t.Errorf("User = %q, want %q", got.User, tt.wantUser)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	fa := false

	tests := []struct {
		name    string
		in      SecurityProfile
		wantErr bool
	}{
		{
			name:    "zero value passes",
			in:      SecurityProfile{},
			wantErr: false,
		},
		{
			name:    "default passes",
			in:      Default(),
			wantErr: false,
		},
		{
			name:    "user=root without AllowRoot fails",
			in:      SecurityProfile{User: "root"},
			wantErr: true,
		},
		{
			name:    "user=0 without AllowRoot fails",
			in:      SecurityProfile{User: "0"},
			wantErr: true,
		},
		{
			name:    "user=0:0 without AllowRoot fails",
			in:      SecurityProfile{User: "0:0"},
			wantErr: true,
		},
		{
			name:    "user=root with AllowRoot passes",
			in:      SecurityProfile{User: "root", AllowRoot: true},
			wantErr: false,
		},
		{
			name:    "NoNewPrivileges=false without AllowRoot fails",
			in:      SecurityProfile{NoNewPrivileges: &fa},
			wantErr: true,
		},
		{
			name:    "NoNewPrivileges=false with AllowRoot passes",
			in:      SecurityProfile{NoNewPrivileges: &fa, AllowRoot: true},
			wantErr: false,
		},
		{
			name:    "non-root user passes",
			in:      SecurityProfile{User: "1000:1000"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%+v) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

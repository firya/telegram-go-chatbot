package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

func TestUserFullName(t *testing.T) {
	tests := []struct {
		name string
		user gotgbot.User
		want string
	}{
		{"only first name", gotgbot.User{FirstName: "Иван"}, "Иван"},
		{"first and last", gotgbot.User{FirstName: "Иван", LastName: "Пупкин"}, "Иван Пупкин"},
		{"empty user", gotgbot.User{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserFullName(&tt.user); got != tt.want {
				t.Errorf("UserFullName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUserName(t *testing.T) {
	withUsername := gotgbot.User{Id: 1, FirstName: "Иван", LastName: "Пупкин", Username: "vpupkin"}
	if got := UserName(&withUsername); got != "vpupkin" {
		t.Errorf("UserName() = %q, want %q", got, "vpupkin")
	}

	withoutUsername := gotgbot.User{Id: 1, FirstName: "Иван", LastName: "Пупкин"}
	if got := UserName(&withoutUsername); got != "Иван Пупкин" {
		t.Errorf("UserName() = %q, want fallback to full name %q", got, "Иван Пупкин")
	}
}

func TestMentionUser(t *testing.T) {
	user := gotgbot.User{Id: 42, FirstName: "Иван", LastName: "Пупкин"}
	want := `<a href="tg://user?id=42">Иван Пупкин</a>`
	if got := MentionUser(&user); got != want {
		t.Errorf("MentionUser() = %q, want %q", got, want)
	}
}

func TestRandInt(t *testing.T) {
	tests := []struct {
		name     string
		min, max int
	}{
		{"positive range", 1, 5},
		{"single value", 7, 7},
		{"negative range", -3, 3},
		{"cross zero", -10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make(map[int]bool)
			for i := 0; i < 1000; i++ {
				v := RandInt(tt.min, tt.max)
				if v < tt.min || v > tt.max {
					t.Fatalf("RandInt(%d, %d) = %d, out of range", tt.min, tt.max, v)
				}
				seen[v] = true
			}
			// For small ranges all values should appear at least once in 1000 draws.
			if tt.max-tt.min < 10 && len(seen) != tt.max-tt.min+1 {
				t.Errorf("RandInt(%d, %d): only %d of %d values seen in 1000 draws",
					tt.min, tt.max, len(seen), tt.max-tt.min+1)
			}
		})
	}
}

func TestStringInSlice(t *testing.T) {
	list := []string{"a", "b", "c"}
	if !StringInSlice("b", list) {
		t.Error("StringInSlice(\"b\", list) = false, want true")
	}
	if StringInSlice("d", list) {
		t.Error("StringInSlice(\"d\", list) = true, want false")
	}
	if StringInSlice("a", nil) {
		t.Error("StringInSlice(\"a\", nil) = true, want false")
	}
}

func TestIsAdmin(t *testing.T) {
	oldConfig := Config
	defer func() { Config = oldConfig }()
	Config = &Configuration{
		Admins: []int64{100, 200},
		Moders: []int64{300},
	}

	if !IsAdmin(100) {
		t.Error("IsAdmin(100) = false, want true")
	}
	if IsAdmin(300) {
		t.Error("IsAdmin(300) = true, want false (moder is not admin)")
	}
	if IsAdmin(999) {
		t.Error("IsAdmin(999) = true, want false")
	}
}

func TestIsAdminOrModer(t *testing.T) {
	oldConfig := Config
	defer func() { Config = oldConfig }()
	Config = &Configuration{
		Admins: []int64{100, 200},
		Moders: []int64{300},
	}

	for _, id := range []int64{100, 200, 300} {
		if !IsAdminOrModer(id) {
			t.Errorf("IsAdminOrModer(%d) = false, want true", id)
		}
	}
	if IsAdminOrModer(999) {
		t.Error("IsAdminOrModer(999) = true, want false")
	}
}

func TestRestrictionTimeMessage(t *testing.T) {
	now := time.Now().Unix()

	// Restriction in effect until the future timestamp -> message with date.
	got := RestrictionTimeMessage(now + 3600)
	if !strings.HasPrefix(got, " до ") {
		t.Errorf("RestrictionTimeMessage(future) = %q, want it to start with \" до \"", got)
	}

	// Already expired (or expiring within 30s) -> empty message.
	if got := RestrictionTimeMessage(now - 10); got != "" {
		t.Errorf("RestrictionTimeMessage(past) = %q, want empty", got)
	}
	if got := RestrictionTimeMessage(now + 10); got != "" {
		t.Errorf("RestrictionTimeMessage(now+10) = %q, want empty (within 30s grace)", got)
	}
}

func TestGetStartOfDay(t *testing.T) {
	got := GetStartOfDay()

	now := time.Now().Local()
	want := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Unix()

	if got != want {
		t.Errorf("GetStartOfDay() = %d, want local midnight %d", got, want)
	}
}

type marshalInner struct {
	InnerSet   string `json:"inner_set"`
	InnerZero  string `json:"inner_zero"`
	InnerOmit  string `json:"-"`
	innerField string
}

type marshalOuter struct {
	Name    string        `json:"name"`
	Zero    int           `json:"zero"`
	Tagged  string        `json:"tagged_field,omitempty,string"`
	Skipped string        `json:"-"`
	private string
	Nested  marshalInner  `json:"nested"`
	NoTag   bool          `json:""`
	Items   []marshalInner `json:"items"`
}

func TestMarshalOmitEmptyAll(t *testing.T) {
	v := marshalOuter{
		Name:    "test",
		Tagged:  "tag",
		Skipped: "gone",
		private: "gone",
		Nested:  marshalInner{InnerSet: "in", InnerOmit: "gone", innerField: "gone"},
		NoTag:   true,
	}

	data, err := MarshalOmitEmptyAll(v)
	if err != nil {
		t.Fatalf("MarshalOmitEmptyAll() error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// Present fields.
	if got["name"] != "test" {
		t.Errorf(`got["name"] = %v, want "test"`, got["name"])
	}
	if got["tagged_field"] != "tag" {
		t.Errorf(`got["tagged_field"] = %v, want "tag"`, got["tagged_field"])
	}
	if got["NoTag"] != true {
		t.Errorf(`got["NoTag"] = %v, want true (empty json tag falls back to field name)`, got["NoTag"])
	}

	// Zero, json:"-" and unexported fields must be dropped.
	for _, key := range []string{"zero", "Skipped", "private", "gone"} {
		if _, ok := got[key]; ok {
			t.Errorf("key %q should be omitted, got: %s", key, data)
		}
	}

	// Nested struct: recursively stripped, present as an object.
	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf(`got["nested"] = %v, want object`, got["nested"])
	}
	if nested["inner_set"] != "in" {
		t.Errorf(`nested["inner_set"] = %v, want "in"`, nested["inner_set"])
	}
	for _, key := range []string{"inner_zero", "InnerOmit", "innerField"} {
		if _, ok := nested[key]; ok {
			t.Errorf("nested key %q should be omitted, got: %s", key, data)
		}
	}
}

func TestMarshalOmitEmptyAll_PointerAndNonStruct(t *testing.T) {
	// Pointer to struct is handled like the struct itself.
	v := marshalOuter{Name: "ptr"}
	data, err := MarshalOmitEmptyAll(&v)
	if err != nil {
		t.Fatalf("MarshalOmitEmptyAll(ptr) error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if got["name"] != "ptr" {
		t.Errorf(`got["name"] = %v, want "ptr"`, got["name"])
	}

	// Non-struct falls back to standard marshaling.
	data, err = MarshalOmitEmptyAll([]string{"a", "b"})
	if err != nil {
		t.Fatalf("MarshalOmitEmptyAll(slice) error: %v", err)
	}
	if string(data) != `["a","b"]` {
		t.Errorf("MarshalOmitEmptyAll(slice) = %s, want [\"a\",\"b\"]", data)
	}
}

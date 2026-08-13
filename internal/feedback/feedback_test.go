package feedback

import (
	"net/url"
	"strings"
	"testing"
)

func report(body string) Report {
	return Report{
		Category: "bug",
		Body:     body,
		Env:      Env{Version: "v1.2.3", OS: "linux", Desktop: "Hyprland", Language: "tr"},
	}
}

// The title is what anyone triaging sees first, so it carries the category and
// the user's own first line rather than a generic label.
func TestTitleTakesTheFirstLine(t *testing.T) {
	got := report("Mikrofon açılmıyor\n\nBir de şu var…").Title()
	if got != "[bug] Mikrofon açılmıyor" {
		t.Errorf("Title = %q", got)
	}
}

// A title that is a whole paragraph is a title nobody can scan.
func TestTitleIsTrimmed(t *testing.T) {
	long := strings.Repeat("uzun ", 40)
	got := report(long).Title()
	if len([]rune(got)) > maxTitle+len("[bug] ")+1 {
		t.Errorf("Title is %d runes: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Title = %q, want it to show it was cut", got)
	}
}

// An issue called "" is worse than one called "[bug]".
func TestTitleSurvivesABodyWithNoFirstLine(t *testing.T) {
	if got := report("\n\n   \n").Title(); got != "[bug]" {
		t.Errorf("Title = %q, want %q", got, "[bug]")
	}
}

// The diagnostics are separated by a rule so nobody reads them as part of what
// the user wrote — and so the user, who saw that exact line under the box,
// recognises it in the issue.
func TestIssueSeparatesTheDiagnostics(t *testing.T) {
	got := report("Şunu yapamıyorum").Issue()
	want := "Şunu yapamıyorum\n\n---\nv1.2.3 · linux · Hyprland · tr"
	if got != want {
		t.Errorf("Issue =\n%q\nwant\n%q", got, want)
	}
}

// Env is assembled from whatever is actually known: a Windows machine has no
// XDG_CURRENT_DESKTOP, and an empty field must not leave a dangling separator.
func TestEnvLineSkipsWhatIsMissing(t *testing.T) {
	got := Env{Version: "v1.2.3", OS: "windows", Language: "en"}.Line()
	if got != "v1.2.3 · windows · en" {
		t.Errorf("Line = %q", got)
	}
	if strings.Contains(got, "··") || strings.HasSuffix(got, "·") {
		t.Errorf("Line = %q, want no empty slot", got)
	}
}

// Nothing is sent that the user has not been shown: the line the form displays
// is the line that ends up in the issue, so they cannot come apart.
func TestWhatIsShownIsWhatIsSent(t *testing.T) {
	r := report("bir şey")
	if !strings.Contains(r.Issue(), r.Env.Line()) {
		t.Error("the issue body does not contain the line the form shows")
	}
}

// The fallback has to arrive with the form already filled in, or it is just a
// link to a blank page.
func TestBrowserURLCarriesTheReport(t *testing.T) {
	r := report("Mikrofon açılmıyor")
	u, err := url.Parse(r.BrowserURL())
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("title") != r.Title() {
		t.Errorf("title = %q, want %q", q.Get("title"), r.Title())
	}
	if q.Get("body") != r.Issue() {
		t.Errorf("body = %q, want the issue body", q.Get("body"))
	}
	if q.Get("labels") != "bug" {
		t.Errorf("labels = %q", q.Get("labels"))
	}
	if !strings.HasPrefix(r.BrowserURL(), "https://github.com/"+repo+"/issues/new") {
		t.Errorf("URL points somewhere else: %s", r.BrowserURL())
	}
}

// The category becomes a GitHub label and is sent straight through from the
// GUI, so an unknown one would file an issue under a label nobody watches.
func TestValidRejectsAnythingNotOffered(t *testing.T) {
	for _, c := range Categories {
		if !Valid(c) {
			t.Errorf("Valid(%q) = false, but it is on the form", c)
		}
	}
	for _, c := range []string{"", "BUG", "urgent", "bug "} {
		if Valid(c) {
			t.Errorf("Valid(%q) = true", c)
		}
	}
}

// Submitting without a token is the fallback's job, not an HTTP request that
// comes back 401 after the user has already pressed Send.
func TestSubmitRefusesWithoutAToken(t *testing.T) {
	if _, err := Submit(nil, "  ", report("bir şey")); err == nil {
		t.Fatal("Submit with no token returned no error")
	}
}

// An empty report is refused before the network, so a stray click cannot file a
// blank issue.
func TestSubmitRefusesAnEmptyBody(t *testing.T) {
	if _, err := Submit(nil, "token", report("   \n  ")); err == nil {
		t.Fatal("Submit with an empty body returned no error")
	}
}

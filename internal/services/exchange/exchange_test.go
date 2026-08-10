package exchange

import (
	"context"
	"errors"
	"testing"
)

// fakeRates implements rateAPI with canned answers.
type fakeRates struct {
	fiat   map[string]float64 // key "BASE/QUOTE"
	crypto map[string]float64 // key "coin/vs"
	err    error
}

func (f fakeRates) fiatRate(_ context.Context, base, quote string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	v, ok := f.fiat[base+"/"+quote]
	if !ok {
		return 0, errors.New("not found")
	}
	return v, nil
}

func (f fakeRates) cryptoPrice(_ context.Context, coin, vs string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	v, ok := f.crypto[coin+"/"+vs]
	if !ok {
		return 0, errors.New("not found")
	}
	return v, nil
}

func withAPI(api rateAPI) *Exchange { return &Exchange{api: api} }

func TestCurrencyDefaultsToTRY(t *testing.T) {
	e := withAPI(fakeRates{fiat: map[string]float64{"USD/TRY": 34.1234}})
	got, err := e.Execute(context.Background(), ActionCurrency, map[string]string{"base": "usd"})
	if err != nil {
		t.Fatal(err)
	}
	want := "1 dollar is 34,12 Turkish lira."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCurrencyExplicitQuote(t *testing.T) {
	e := withAPI(fakeRates{fiat: map[string]float64{"GBP/USD": 1.27}})
	got, _ := e.Execute(context.Background(), ActionCurrency, map[string]string{"base": "GBP", "quote": "usd"})
	if got != "1 pound is 1,27 dollars." {
		t.Fatalf("got %q", got)
	}
}

func TestCryptoGrouping(t *testing.T) {
	e := withAPI(fakeRates{crypto: map[string]float64{"bitcoin/TRY": 2850000.5}})
	got, _ := e.Execute(context.Background(), ActionCrypto, map[string]string{"coin": "Bitcoin"})
	if got != "Bitcoin is at 2.850.000,50 Turkish lira." {
		t.Fatalf("got %q", got)
	}
}

func TestMissingArgsPrompt(t *testing.T) {
	e := withAPI(fakeRates{})
	if got, _ := e.Execute(context.Background(), ActionCurrency, nil); got != "Which currency do you mean?" {
		t.Errorf("currency missing base: %q", got)
	}
	if got, _ := e.Execute(context.Background(), ActionCrypto, nil); got != "Which cryptocurrency do you mean?" {
		t.Errorf("crypto missing coin: %q", got)
	}
}

func TestAPIErrorIsGraceful(t *testing.T) {
	e := withAPI(fakeRates{err: errors.New("network down")})
	if got, err := e.Execute(context.Background(), ActionCurrency, map[string]string{"base": "USD"}); err != nil || got != "I can't get exchange rates right now." {
		t.Fatalf("currency err path: %q err=%v", got, err)
	}
	if got, err := e.Execute(context.Background(), ActionCrypto, map[string]string{"coin": "bitcoin"}); err != nil || got != "I can't get the price right now." {
		t.Fatalf("crypto err path: %q err=%v", got, err)
	}
}

func TestUnknownAction(t *testing.T) {
	if _, err := withAPI(fakeRates{}).Execute(context.Background(), "exchange.bogus", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[float64]string{
		34.125:     "34,13", // rounds
		0:          "0,00",
		999:        "999,00",
		1000:       "1.000,00",
		2850000.5:  "2.850.000,50",
		1234567.89: "1.234.567,89",
		-5.5:       "-5,50",
		0.999:      "1,00", // frac rounds into whole
	}
	for in, want := range cases {
		if got := formatMoney(in); got != want {
			t.Errorf("formatMoney(%v) = %q, want %q", in, got, want)
		}
	}
}

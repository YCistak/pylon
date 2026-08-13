// Package exchange answers live currency and crypto price questions ("dolar ne
// kadar", "bitcoin kaç lira"). It uses two free, key-less APIs — open.er-api.com
// for fiat rates and CoinGecko for crypto — behind a small interface so tests
// fake the network. It needs no configuration, so it is always registered; the
// default quote currency is the Turkish lira.
package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

const (
	// ActionCurrency reports a fiat rate: `base` per 1 unit in `quote` (default TRY).
	ActionCurrency intent.Action = "exchange.currency"
	// ActionCrypto reports a crypto price: 1 `coin` in `vs` currency (default TRY).
	ActionCrypto intent.Action = "exchange.crypto"
)

// defaultQuote is used when the user doesn't name a target currency.
const defaultQuote = "TRY"

// rateAPI is the slice of the price providers the service uses; a fake
// implements it in tests.
type rateAPI interface {
	fiatRate(ctx context.Context, base, quote string) (float64, error)
	cryptoPrice(ctx context.Context, coin, vs string) (float64, error)
}

// Exchange is the price Service.
type Exchange struct {
	api rateAPI // injected in tests; otherwise the real HTTP client
}

// New builds the service backed by the live HTTP APIs.
func New() *Exchange {
	return &Exchange{api: &httpRates{client: &http.Client{Timeout: 8 * time.Second}}}
}

func (e *Exchange) Name() string { return "exchange" }

func (e *Exchange) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionCurrency,
			Args: []string{"base", "quote"},
			Desc: `"exchange.currency": live fiat exchange rate. "base" is the ISO 4217 code being priced (USD, EUR, GBP, ...); "quote" is the target code (default TRY). Use for "dolar ne kadar" (base:USD), "euro kaç lira" (base:EUR), "1 sterlin kaç dolar" (base:GBP, quote:USD).`,
		},
		{
			Name: ActionCrypto,
			Args: []string{"coin", "vs"},
			Desc: `"exchange.crypto": live cryptocurrency price. "coin" is the lowercase CoinGecko id (bitcoin, ethereum, solana, dogecoin, ...); "vs" is the target currency code (default TRY). Use for "bitcoin kaç para" (coin:bitcoin), "ethereum kaç dolar" (coin:ethereum, vs:USD).`,
		},
	}
}

func (e *Exchange) Execute(ctx context.Context, action intent.Action, args map[string]string) (string, error) {
	switch action {
	case ActionCurrency:
		base := code(args["base"])
		if base == "" {
			return i18n.T("exchange.which_currency"), nil
		}
		quote := codeOr(args["quote"], defaultQuote)
		rate, err := e.api.fiatRate(ctx, base, quote)
		if err != nil {
			return i18n.T("exchange.rate_unavailable"), nil
		}
		return i18n.T("exchange.rate", currencyName(base, 1), i18n.Money(rate), currencyName(quote, rate)), nil
	case ActionCrypto:
		coin := strings.ToLower(strings.TrimSpace(args["coin"]))
		if coin == "" {
			return i18n.T("exchange.which_coin"), nil
		}
		vs := codeOr(args["vs"], defaultQuote)
		price, err := e.api.cryptoPrice(ctx, coin, vs)
		if err != nil {
			return i18n.T("exchange.price_unavailable"), nil
		}
		return i18n.T("exchange.price", coinName(coin), i18n.Money(price), currencyName(vs, price)), nil
	default:
		return "", fmt.Errorf("exchange: unknown action %q", action)
	}
}

// code normalizes a currency code to upper-case, trimmed.
func code(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func codeOr(s, fallback string) string {
	if c := code(s); c != "" {
		return c
	}
	return fallback
}

// currencyName speaks a code the way a person would ("USD" → "dollars",
// "dolar", "доллара"). The names live in the catalogs rather than a table here,
// because in the languages Pylon speaks they are ordinary nouns that decline.
func currencyName(codeUpper string, amount float64) string {
	if key := "currency." + codeUpper; i18n.Has(key) {
		return i18n.FormFloat(key, amount)
	}
	// An unlisted code is spoken as the code itself — wrong-sounding but never
	// wrong, which is the right trade for money.
	return codeUpper
}

// coinName title-cases a CoinGecko id for display ("bitcoin" → "Bitcoin").
func coinName(id string) string {
	if id == "" {
		return id
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

// httpRates is the live implementation of rateAPI.
type httpRates struct{ client *http.Client }

func (h *httpRates) fiatRate(ctx context.Context, base, quote string) (float64, error) {
	// Escaped, because base is whatever the model produced from the sentence:
	// code() only trims and upper-cases it. The host is fixed either way, so
	// this cannot reach another server — but a stray "?" or "../" would quietly
	// request a different endpoint of this one.
	endpoint := "https://open.er-api.com/v6/latest/" + url.PathEscape(base)
	var body struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := h.getJSON(ctx, endpoint, &body); err != nil {
		return 0, err
	}
	if body.Result != "success" {
		return 0, fmt.Errorf("exchange: invalid currency %q", base)
	}
	rate, ok := body.Rates[quote]
	if !ok {
		return 0, fmt.Errorf("exchange: no rate for %s/%s", base, quote)
	}
	return rate, nil
}

func (h *httpRates) cryptoPrice(ctx context.Context, coin, vs string) (float64, error) {
	// Same reasoning as fiatRate, in a query string: an unescaped "&" in coin
	// would append parameters of the model's choosing.
	q := url.Values{}
	q.Set("ids", coin)
	q.Set("vs_currencies", strings.ToLower(vs))
	endpoint := "https://api.coingecko.com/api/v3/simple/price?" + q.Encode()
	var body map[string]map[string]float64
	if err := h.getJSON(ctx, endpoint, &body); err != nil {
		return 0, err
	}
	vsRates, ok := body[coin]
	if !ok {
		return 0, fmt.Errorf("exchange: unknown coin %q", coin)
	}
	price, ok := vsRates[strings.ToLower(vs)]
	if !ok {
		return 0, fmt.Errorf("exchange: no %s price for %q", vs, coin)
	}
	return price, nil
}

func (h *httpRates) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("exchange: %s → %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

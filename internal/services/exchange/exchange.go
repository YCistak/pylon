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
	"strconv"
	"strings"
	"time"

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
			return "Hangi para birimini soruyorsun?", nil
		}
		quote := codeOr(args["quote"], defaultQuote)
		rate, err := e.api.fiatRate(ctx, base, quote)
		if err != nil {
			return "Kur bilgisini şu an alamadım.", nil
		}
		return fmt.Sprintf("1 %s %s %s.", currencyName(base), formatMoney(rate), currencyName(quote)), nil
	case ActionCrypto:
		coin := strings.ToLower(strings.TrimSpace(args["coin"]))
		if coin == "" {
			return "Hangi kripto parayı soruyorsun?", nil
		}
		vs := codeOr(args["vs"], defaultQuote)
		price, err := e.api.cryptoPrice(ctx, coin, vs)
		if err != nil {
			return "Fiyatı şu an alamadım.", nil
		}
		return fmt.Sprintf("%s %s %s.", coinName(coin), formatMoney(price), currencyName(vs)), nil
	default:
		return "", fmt.Errorf("exchange: bilinmeyen aksiyon %q", action)
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

// formatMoney renders a price for Turkish speech/display: thousands grouped with
// '.', two decimals after a ',' ("2.850.000,50", "34,12").
func formatMoney(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := int64(v)
	frac := int64((v-float64(whole))*100 + 0.5)
	if frac == 100 { // rounding carried into the integer part
		whole++
		frac = 0
	}
	s := groupThousands(strconv.FormatInt(whole, 10)) + "," + fmt.Sprintf("%02d", frac)
	if neg {
		return "-" + s
	}
	return s
}

func groupThousands(digits string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	lead := n % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
		if n > lead {
			b.WriteByte('.')
		}
	}
	for i := lead; i < n; i += 3 {
		b.WriteString(digits[i : i+3])
		if i+3 < n {
			b.WriteByte('.')
		}
	}
	return b.String()
}

// currencyName maps common ISO codes to a Turkish spoken name, falling back to
// the code itself for anything unlisted.
func currencyName(codeUpper string) string {
	if n, ok := trCurrency[codeUpper]; ok {
		return n
	}
	return codeUpper
}

var trCurrency = map[string]string{
	"TRY": "Türk lirası",
	"USD": "dolar",
	"EUR": "euro",
	"GBP": "sterlin",
	"JPY": "yen",
	"CHF": "İsviçre frangı",
	"AUD": "Avustralya doları",
	"CAD": "Kanada doları",
	"RUB": "ruble",
	"SAR": "Suudi riyali",
	"AED": "dirhem",
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
	url := "https://open.er-api.com/v6/latest/" + base
	var body struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := h.getJSON(ctx, url, &body); err != nil {
		return 0, err
	}
	if body.Result != "success" {
		return 0, fmt.Errorf("exchange: geçersiz para birimi %q", base)
	}
	rate, ok := body.Rates[quote]
	if !ok {
		return 0, fmt.Errorf("exchange: kur bulunamadı %s/%s", base, quote)
	}
	return rate, nil
}

func (h *httpRates) cryptoPrice(ctx context.Context, coin, vs string) (float64, error) {
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=%s", coin, strings.ToLower(vs))
	var body map[string]map[string]float64
	if err := h.getJSON(ctx, url, &body); err != nil {
		return 0, err
	}
	vsRates, ok := body[coin]
	if !ok {
		return 0, fmt.Errorf("exchange: kripto bulunamadı %q", coin)
	}
	price, ok := vsRates[strings.ToLower(vs)]
	if !ok {
		return 0, fmt.Errorf("exchange: %s karşılığı bulunamadı %q", vs, coin)
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

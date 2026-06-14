package swpc

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes swpc as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/swpc-cli/swpc"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then routes
// swpc:// URIs to the operations Register installs. The same Domain also
// builds the standalone swpc binary (see cli.NewApp), so the binary and a
// host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the swpc driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "swpc",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "swpc",
			Short:  "A command line for NOAA Space Weather Prediction Center.",
			Long: `A command line for NOAA Space Weather Prediction Center.

swpc reads real-time and recent space weather data from the NOAA SWPC public
API, shapes it into clean records, and prints output that pipes into the rest
of your tools. No API key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/swpc-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "kindex",
		Group:   "read",
		Summary: "Recent planetary K-index readings",
	}, getKIndex)

	kit.Handle(app, kit.OpMeta{
		Name:    "solarwind",
		Group:   "read",
		Summary: "Current solar wind speed",
	}, getSolarWind)

	kit.Handle(app, kit.OpMeta{
		Name:    "alerts",
		Group:   "read",
		Summary: "Space weather alerts and watches",
	}, getAlerts)

	kit.Handle(app, kit.OpMeta{
		Name:    "flares",
		Group:   "read",
		Summary: "Recent solar flares (7 days)",
	}, getFlares)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type kindexInput struct {
	Limit  int     `kit:"flag,inherit" help:"max K-index records"`
	Client *Client `kit:"inject"`
}

type solarwindInput struct {
	Client *Client `kit:"inject"`
}

type alertsInput struct {
	Limit  int     `kit:"flag,inherit" help:"max alerts"`
	Client *Client `kit:"inject"`
}

type flaresInput struct {
	Limit  int     `kit:"flag,inherit" help:"max flares"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getKIndex(ctx context.Context, in kindexInput, emit func(*KIndex) error) error {
	records, err := in.Client.GetKIndex(ctx, in.Limit)
	if err != nil {
		return err
	}
	for i := range records {
		if err := emit(&records[i]); err != nil {
			return err
		}
	}
	return nil
}

func getSolarWind(ctx context.Context, in solarwindInput, emit func(*SolarWind) error) error {
	sw, err := in.Client.GetSolarWind(ctx)
	if err != nil {
		return err
	}
	return emit(sw)
}

func getAlerts(ctx context.Context, in alertsInput, emit func(*SpaceAlert) error) error {
	alerts, err := in.Client.GetAlerts(ctx, in.Limit)
	if err != nil {
		return err
	}
	for i := range alerts {
		if err := emit(&alerts[i]); err != nil {
			return err
		}
	}
	return nil
}

func getFlares(ctx context.Context, in flaresInput, emit func(*SolarFlare) error) error {
	flares, err := in.Client.GetFlares(ctx, in.Limit)
	if err != nil {
		return err
	}
	for i := range flares {
		if err := emit(&flares[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns an accepted input into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	// Dataset names
	switch input {
	case "kindex", "solarwind", "alerts", "flares":
		return "dataset", input, nil
	}
	// Solar flare class (e.g. "M2.1", "X1.5", "B3", "C4.2")
	if len(input) > 0 && (input[0] == 'B' || input[0] == 'C' || input[0] == 'M' || input[0] == 'X') {
		return "class", input, nil
	}
	// Date-like strings
	if len(input) == 10 && input[4] == '-' && input[7] == '-' {
		return "date", input, nil
	}
	return "", "", errs.Usage("unrecognized swpc reference: %q", input)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "dataset" {
		return "", errs.Usage("swpc has no resource type %q", uriType)
	}
	return "https://www.swpc.noaa.gov/products/" + id, nil
}

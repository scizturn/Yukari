package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/notify"
	"github.com/kyou-id/yukari/internal/runreport"
)

// campaign is one subcommand. It returns the reader's error instead of calling
// log.Fatalf: an os.Exit inside the campaign would skip the Discord report, and a
// failed cron run is exactly the one we most want to hear about.
type campaign struct {
	title string
	run   func(ctx context.Context, run *runreport.Run) error
}

var campaigns = map[string]campaign{
	"birthday":            {title: "Birthday", run: runBirthday},
	"anniversary":         {title: "Anniversary", run: runAnniversary},
	"leftover-cart":       {title: "Leftover Cart", run: runLeftoverCart},
	"discounted-wishlist": {title: "Discounted Wishlist", run: runDiscountedWishlist},
	"wishlist-back-in":    {title: "Wishlist Back In", run: runWishlistBackIn},
	"winback":             {title: "Winback", run: runWinback},
	"po-ready":            {title: "PO Ready", run: runPoReady},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: yukari <campaign>")
		fmt.Fprintf(os.Stderr, "campaigns: %s\n", strings.Join(campaignNames(), ", "))
		os.Exit(1)
	}

	name := os.Args[1]
	selected, ok := campaigns[name]
	if !ok {
		log.Fatalf("unknown campaign: %q  (valid: %s)", name, strings.Join(campaignNames(), ", "))
	}

	ctx := context.Background()
	cfg := config.Load()

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}

	run := runreport.New(name, selected.title, time.Now().In(location))

	// A panic is the one failure nobody sees: the container dies, Coolify records a
	// non-zero exit, and no email is ever sent. Report it, then let it crash.
	defer func() {
		if recovered := recover(); recovered != nil {
			report(ctx, cfg, run, time.Now().In(location), fmt.Errorf("panic: %v", recovered))
			panic(recovered)
		}
	}()

	runErr := selected.run(ctx, run)
	report(ctx, cfg, run, time.Now().In(location), runErr)

	if runErr != nil {
		log.Fatalf("%s reader failed: %v", name, runErr)
	}
	log.Printf("yukari enqueued %d %s email job(s)", run.Queued, name)
}

// report posts the run summary to Discord. A webhook that is down must not turn a
// successful cron run into a failed one, so its error is logged, not returned.
func report(ctx context.Context, cfg config.Config, run *runreport.Run, finishedAt time.Time, runErr error) {
	discord := notify.DiscordLogger{
		WebhookURL: cfg.DiscordWebhookURL,
		Enabled:    cfg.DiscordEnabled,
	}
	if err := discord.Log(ctx, run.Message(finishedAt, runErr)); err != nil {
		log.Printf("discord run report failed: %v", err)
	}
}

func campaignNames() []string {
	names := make([]string, 0, len(campaigns))
	for name := range campaigns {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

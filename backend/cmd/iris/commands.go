package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/data"
)

// runCommand dispatches an admin subcommand (positional args after the flags).
// Returns the process exit code. Unknown commands print usage and exit non-zero.
func runCommand(ctx context.Context, cfg *conf.Config, log *slog.Logger, args []string) int {
	switch args[0] {
	case "clear-suppressions":
		fs := flag.NewFlagSet("clear-suppressions", flag.ExitOnError)
		force := fs.Bool("yes", false, "skip the interactive confirmation prompt")
		_ = fs.Parse(args[1:])
		return runClearSuppressions(ctx, cfg, log, *force)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\nAvailable commands:\n  clear-suppressions [-yes]   Delete every suppression entry (database + Redis)\n", args[0])
		return 2
	}
}

// runClearSuppressions permanently deletes every suppression entry from Postgres
// and flushes the Redis live-suppression cache. It reuses the server's config so
// it targets the same database and Redis. KumoMTA's memoized lookup (60s TTL)
// reflects the empty list within a minute, so no restart is required.
func runClearSuppressions(ctx context.Context, cfg *conf.Config, log *slog.Logger, force bool) int {
	if !force && !confirm("This permanently deletes ALL suppression entries from the database and Redis.") {
		fmt.Println("aborted.")
		return 1
	}

	db, dbCleanup, err := data.NewDB(ctx, cfg.Data.Database)
	if err != nil {
		log.Error("clear-suppressions: open database", "error", err.Error())
		return 1
	}
	defer dbCleanup()

	// Attach the Redis cache when configured; without it the clear is DB-only
	// (SuppressionCache methods are nil-safe).
	var suppCache *data.SuppressionCache
	if cfg.Data.Redis.Addr != "" {
		streams, streamsCleanup, serr := data.NewStreams(ctx, cfg.Data.Redis)
		if serr != nil {
			log.Error("clear-suppressions: open redis", "error", serr.Error())
			return 1
		}
		defer streamsCleanup()
		suppCache = data.NewSuppressionCache(streams.Client)
	}

	repo := data.NewDomainSafetyRepo(db).WithSuppressionCache(suppCache, nil)
	n, err := repo.ClearAllSuppressions(ctx)
	if err != nil {
		log.Error("clear-suppressions: failed", "error", err.Error())
		return 1
	}

	fmt.Printf("cleared %d suppression entries (database + Redis).\n", n)
	fmt.Println("KumoMTA's memoized lookup (60s TTL) will reflect the empty list within a minute — no restart needed.")
	return 0
}

// confirm prompts on stdin and returns true only when the operator types "yes".
func confirm(warning string) bool {
	fmt.Println(warning)
	fmt.Print("Type 'yes' to continue: ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(answer), "yes")
}

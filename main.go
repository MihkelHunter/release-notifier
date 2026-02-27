package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/MihkelHunter/release-notifier/internal/config"
	"github.com/MihkelHunter/release-notifier/internal/email"
	"github.com/MihkelHunter/release-notifier/internal/markdown"
	"github.com/MihkelHunter/release-notifier/internal/outlook"
	"github.com/MihkelHunter/release-notifier/internal/recipients"
)

func main() {
	var (
		notesFile  = flag.String("notes", "", "Path to markdown release notes file (required)")
		env        = flag.String("env", "production", "Target environment (e.g. production, staging)")
		configFile = flag.String("config", "config.yaml", "Path to config file")
		csvFile    = flag.String("recipients", "recipients.csv", "Path to recipients CSV file")
		dryRun     = flag.Bool("dry-run", false, "Print email content without sending or opening")
		version    = flag.String("version", "", "Release version override (e.g. v1.2.3)")
		useOutlook = flag.Bool("outlook", false, "Open Outlook compose window instead of sending via Graph API")
		autoSend   = flag.Bool("auto-send", false, "With --outlook: send immediately without showing compose window")
		ccFlag     = flag.String("cc", "", "Comma-separated CC addresses (optional, used with --outlook)")
	)
	flag.Parse()

	if *notesFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --notes flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Parse markdown release notes + extract tags
	parsed, err := markdown.ParseReleaseNotes(*notesFile)
	if err != nil {
		log.Fatalf("Failed to parse release notes: %v", err)
	}
	if *version != "" {
		parsed.Version = *version
	}

	// Config is required for Graph mode; for Outlook mode load leniently
	var cfg *config.Config
	if *useOutlook {
		cfg, err = config.LoadMinimal(*configFile)
		if err != nil {
			cfg = config.DefaultConfig()
		}
	} else {
		cfg, err = config.Load(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	}

	// Resolve recipients from CSV + tags
	recipientList, err := recipients.Resolve(*csvFile, parsed.Tags, *env, cfg)
	if err != nil {
		log.Fatalf("Failed to resolve recipients: %v", err)
	}
	if len(recipientList) == 0 {
		log.Fatal("No recipients resolved. Check your tags and recipients.csv.")
	}

	fmt.Printf("📦 Release:     %s\n", parsed.Version)
	fmt.Printf("🌍 Environment: %s\n", *env)
	fmt.Printf("🏷️  Tags found:  %v\n", parsed.Tags)
	fmt.Printf("📧 Recipients (%d):\n", len(recipientList))
	for _, r := range recipientList {
		fmt.Printf("   - %s <%s>\n", r.Name, r.Email)
	}

	// Build email HTML
	msg, err := email.Build(parsed, *env, cfg, recipientList)
	if err != nil {
		log.Fatalf("Failed to build email: %v", err)
	}

	if *dryRun {
		fmt.Println("\n--- DRY RUN: Subject ---")
		fmt.Println(msg.Subject)
		fmt.Println("\n--- DRY RUN: HTML Body ---")
		fmt.Println(msg.Body)
		fmt.Println("--- End ---")
		fmt.Println("\n✅ Dry run complete. No email sent.")
		return
	}

	// ── Outlook COM mode ──────────────────────────────────────────────────────
	if *useOutlook {
		toAddrs := make([]string, len(recipientList))
		for i, r := range recipientList {
			if r.Name != "" {
				toAddrs[i] = fmt.Sprintf("%s <%s>", r.Name, r.Email)
			} else {
				toAddrs[i] = r.Email
			}
		}

		var ccAddrs []string
		if *ccFlag != "" {
			for _, addr := range strings.Split(*ccFlag, ",") {
				if trimmed := strings.TrimSpace(addr); trimmed != "" {
					ccAddrs = append(ccAddrs, trimmed)
				}
			}
		}

		opts := outlook.DraftOptions{
			Subject:  msg.Subject,
			HTMLBody: msg.Body,
			To:       toAddrs,
			CC:       ccAddrs,
			AutoSend: *autoSend,
		}

		if *autoSend {
			fmt.Println("📨 Sending via Outlook...")
		} else {
			fmt.Println("📝 Opening Outlook compose window...")
		}

		if err := outlook.OpenDraft(opts); err != nil {
			log.Fatalf("Failed to open Outlook: %v", err)
		}

		if *autoSend {
			fmt.Println("✅ Email sent via Outlook!")
		} else {
			fmt.Println("✅ Compose window opened — review and hit Send.")
		}
		return
	}

	// ── Microsoft Graph API mode ──────────────────────────────────────────────
	fmt.Println("📨 Sending via Microsoft Graph API...")
	sender := email.NewGraphSender(cfg)
	if err := sender.Send(msg); err != nil {
		log.Fatalf("Failed to send email: %v", err)
	}
	fmt.Println("✅ Release notification sent successfully!")
}

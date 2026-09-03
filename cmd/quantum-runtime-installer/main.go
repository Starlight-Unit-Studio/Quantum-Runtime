package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/buildinfo"
	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/installer"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	manager := &installer.Manager{
		Layout:      installer.NewLayout("/"),
		Version:     buildinfo.Version,
		RequireRoot: true,
	}

	switch os.Args[1] {
	case "preflight":
		fs := flag.NewFlagSet("preflight", flag.ExitOnError)
		jsonOutput := fs.Bool("json", false, "print machine-readable JSON")
		_ = fs.Parse(os.Args[2:])
		result, err := manager.Preflight(ctx)
		fatalIf(err)
		if *jsonOutput {
			printJSON(result)
			return
		}
		fmt.Printf("systemd: %t\n", result.SystemdAvailable)
		fmt.Printf("runtime: %s\n", result.Runtime.Ownership)
		fmt.Printf("ollama: %s\n", result.Ollama.Ownership)
		fmt.Printf("can_install: %t\n", result.CanInstall)
		for _, warning := range result.Warnings {
			fmt.Printf("warning: %s\n", warning)
		}
	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		jsonOutput := fs.Bool("json", false, "print machine-readable JSON")
		_ = fs.Parse(os.Args[2:])
		result := manager.Status(ctx)
		if *jsonOutput {
			printJSON(result)
			return
		}
		fmt.Printf("runtime: %s available=%t health=%t ready=%t\n", result.Runtime.Ownership, result.Runtime.Available, result.Health, result.Ready)
		fmt.Printf("ollama: %s available=%t\n", result.Ollama.Ownership, result.Ollama.Available)
	case "install", "repair":
		fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
		binary := fs.String("runtime-binary", "./quantum-runtime", "path to the Quantum Runtime binary to install")
		noStart := fs.Bool("no-start", false, "install files without enabling or starting the service")
		_ = fs.Parse(os.Args[2:])
		options := installer.InstallOptions{RuntimeBinary: *binary, StartService: !*noStart}
		var err error
		if os.Args[1] == "repair" {
			err = manager.Repair(ctx, options)
		} else {
			err = manager.Install(ctx, options)
		}
		fatalIf(err)
		fmt.Printf("Quantum Runtime %s completed successfully.\n", os.Args[1])
	case "uninstall":
		fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
		purgeConfig := fs.Bool("purge-managed-config", false, "remove the Runtime config only if the installer originally created it")
		_ = fs.Parse(os.Args[2:])
		fatalIf(manager.Uninstall(ctx, *purgeConfig))
		fmt.Println("Quantum Runtime uninstall completed. Existing Ollama and model data were not modified.")
	case "coreui-profile":
		fs := flag.NewFlagSet("coreui-profile", flag.ExitOnError)
		mode := fs.String("mode", "runtime", "profile mode: runtime or ollama")
		_ = fs.Parse(os.Args[2:])
		profile, err := installer.CoreUIProfile(*mode)
		fatalIf(err)
		fmt.Print(profile)
	case "version":
		fmt.Println(buildinfo.Version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: quantum-runtime-installer <preflight|status|install|repair|uninstall|coreui-profile|version> [options]")
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "quantum-runtime-installer: %v\n", err)
	os.Exit(1)
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fatalIf(err)
	}
}

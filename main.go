package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
	"github.com/webpro/reveal-md/internal/print"
	"github.com/webpro/reveal-md/internal/server"
	"github.com/webpro/reveal-md/internal/static"
)

//go:embed help.txt
var helpText string

const version = "6.1.4"

var alias = map[string][]string{
	"h": {"help"},
	"s": {"separator"},
	"S": {"vertical-separator"},
	"t": {"theme"},
	"V": {"version"},
}

func main() {
	argv := os.Args[1:]
	args := jsutil.ParseArgv(argv, jsutil.YargsOptions{Alias: alias})

	positional, _ := args.Get("_").([]any)
	hasPath := len(positional) > 0 && jsutil.Truthy(positional[0])

	if jsutil.Truthy(args.Get("version")) {
		fmt.Println(version)
		return
	}

	if !hasPath && !jsutil.Truthy(args.Get("static")) {
		fmt.Println(helpText)
		return
	}

	if err := run(argv, args); err != nil {
		fmt.Fprintln(os.Stderr, inspect(err))
		os.Exit(1)
	}
}

func run(argv []string, args *jsutil.Object) error {
	cwd, err := jsutil.PhysicalCwd()
	if err != nil {
		return err
	}
	cfg, err := config.Load(argv, cwd)
	if err != nil {
		return err
	}

	switch {
	case cfg.HasStatic():
		return exportStatic(cfg, args)
	case jsutil.Truthy(args.Get("print")):
		return printPDF(cfg, args)
	default:
		return serve(cfg, args)
	}
}

func exportStatic(cfg *config.Config, args *jsutil.Object) error {
	var srv *server.Server
	if jsutil.Truthy(args.Get("featuredSlide")) {
		started, err := server.Start(cfg)
		if err != nil {
			return err
		}
		srv = started
	}
	if err := static.Export(cfg); err != nil {
		return err
	}
	if srv != nil {
		srv.Close()
	}
	return nil
}

func printPDF(cfg *config.Config, args *jsutil.Object) error {
	srv, err := server.Start(cfg)
	if err != nil {
		return err
	}
	if err := print.Print(cfg, srv.URL, args.Get("print"), args.Get("printSize")); err != nil {
		return err
	}
	srv.Close()
	return nil
}

func serve(cfg *config.Config, args *jsutil.Object) error {
	srv, err := server.Start(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("The slides are at %s\n", srv.URL)

	if !jsutil.Truthy(args.Get("disableAutoOpen")) {
		openBrowser(srv.URL)
	}

	waitForInterrupt()
	fmt.Println("Received SIGINT, closing gracefully.")
	srv.Close()
	exitProcess(128)
	return nil
}

var waitForInterrupt = func() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGINT)
	<-interrupt
}

var exitProcess = os.Exit

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		jsutil.Debug(err)
		return
	}
	go func() { _ = cmd.Wait() }()
}

func inspect(err error) string {
	return jsutil.Inspect(err)
}

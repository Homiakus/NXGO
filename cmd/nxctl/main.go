package main

import (
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "runtime"
    "strings"
    "time"
)

func main() {
    if err := run(os.Args[1:]); err != nil {
        fmt.Fprintln(os.Stderr, "nxctl:", err)
        os.Exit(1)
    }
}

func run(args []string) error {
    if len(args) < 2 || args[0] != "test" {
        return errors.New("usage: nxctl test <fast|nx|matrix|chaos|soak|perf>")
    }
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
    defer cancel()

    switch args[1] {
    case "fast":
        if err := runCmd(ctx, "go", "test", "-race", "./..."); err != nil { return err }
        if err := runCmd(ctx, "go", "vet", "./..."); err != nil { return err }
        return runCmd(ctx, "go", "run", "./cmd/invariantcheck")
    case "nx":
        return runRealNX(ctx, os.Getenv("NXGO_NX_HOME"))
    case "matrix":
        raw := strings.TrimSpace(os.Getenv("NXGO_NX_MATRIX"))
        if raw == "" { return errors.New("NXGO_NX_MATRIX is required; use semicolon-separated NX installation roots") }
        for _, home := range strings.Split(raw, ";") {
            if err := runRealNX(ctx, strings.TrimSpace(home)); err != nil { return fmt.Errorf("matrix entry %q: %w", home, err) }
        }
        return nil
    case "chaos":
        return runCmd(ctx, "go", "test", "./internal/fakeagent", "-run", "Chaos", "-count=1")
    case "soak":
        return runCmd(ctx, "go", "test", "./internal/fakeagent", "-run", "Soak", "-count=1")
    case "perf":
        return runCmd(ctx, "go", "test", "./internal/fakeagent", "-run", "^$", "-bench", ".", "-benchmem")
    default:
        return fmt.Errorf("unknown test loop %q", args[1])
    }
}

func runRealNX(ctx context.Context, home string) error {
    if runtime.GOOS != "windows" { return errors.New("real NX loop requires Windows") }
    if strings.TrimSpace(home) == "" { return errors.New("NXGO_NX_HOME is required; refusing to report a real-NX pass without an explicit installation") }
    old := os.Getenv("NXGO_NX_HOME")
    if err := os.Setenv("NXGO_NX_HOME", home); err != nil { return err }
    defer os.Setenv("NXGO_NX_HOME", old)
    return runCmd(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/nx-real-smoke.ps1")
}

func runCmd(ctx context.Context, name string, args ...string) error {
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin
    if err := cmd.Run(); err != nil { return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err) }
    return nil
}

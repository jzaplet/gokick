---
layout: 'page'
uri: '/framework/entry-point'
position: 1
slug: 'framework-entry-point'
parent: 'framework'
navTitle: 'Entry Point'
title: 'Entry Point'
description: 'Vstup do aplikace – main.go, Cobra CLI, příkaz serve, lifecycle.'
---

# Entry Point


## main.go

Vstupní bod aplikace. Inicializuje DI kontejner a spustí CLI:

```go
func main() {
    app := di_container.CreateApplication()
    err := app.Run()
    if err != nil {
        os.Exit(1)
    }
}
```


## Application lifecycle

`application.go` definuje struct `Application` s metodou `Run()`. Wire DI ho vytvoří se všemi závislostmi.

```go
type Application struct {
    rootCmd *cobra.Command
}

func (a *Application) Run() error {
    return a.rootCmd.Execute()
}
```


## Cobra CLI

Dva soubory v `console/`:

- **`root.go`** – root command, definuje název a verzi
- **`serve.go`** – příkaz `serve`, spustí HTTP server

```
filmshes [command]

Available Commands:
  serve       Spustí HTTP server
  help        Nápověda
```


### Příkaz serve

```go
// console/serve.go
type ServeCommand struct {
    server *server.Server
}

func (c *ServeCommand) Command() *cobra.Command {
    return &cobra.Command{
        Use:   "serve",
        Short: "Spustí HTTP server",
        RunE: func(cmd *cobra.Command, args []string) error {
            return c.server.Start()
        },
    }
}
```


## Spuštění

```bash
./bin/filmshes serve
# Nebo:
make serve
```

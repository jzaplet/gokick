---
layout: 'page'
uri: '/framework/presentation/console'
position: 3
slug: 'framework-presentation-console'
parent: 'framework-presentation'
navTitle: 'Console'
title: 'Console'
description: 'Cobra CLI -- root command, serve a seed subcommand.'
---

# Console

## Proč

CLI je druhý vstupní bod aplikace vedle HTTP serveru. Cobra umožňuje snadno přidávat další příkazy (migrace, seedy, one-off skripty) bez změny serveru.

## Jak

Balíček `presentation/console/`. Root command `app` s podpříkazy:

```
app [command]

Available Commands:
  serve       Spustí HTTP server
  seed        Naplní databázi výchozími daty
  help        Nápověda
```

### Serve command

```go
// presentation/console/serve.go

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

Spuštění:

```bash
./bin/app serve
# Nebo:
make serve
```

### Seed command

```go
// presentation/console/seed.go

type SeedCommand struct {
    seeder shared.Seeder
}

func (c *SeedCommand) Command() *cobra.Command {
    return &cobra.Command{
        Use:   "seed",
        Short: "Naplní databázi výchozími daty",
        RunE: func(cmd *cobra.Command, args []string) error {
            return c.seeder.Seed(cmd.Context())
        },
    }
}
```

Spuštění:

```bash
./bin/app seed
```


## Detaily

- Root command (`root.go`) registruje všechny subcommandy.
- Každý příkaz dostává závislosti přes DI (Wire) a závisí na doménové interfaces (např. `shared.Seeder`), ne na konkrétní infrastrukturu.
- Nové příkazy se přidávají stejným patternem: struct s `Command() *cobra.Command` metodou, registrace v root commandu.

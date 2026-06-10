---
layout: 'page'
uri: '/framework/presentation/console'
position: 3
slug: 'framework-presentation-console'
parent: 'framework-presentation'
navTitle: 'Console'
title: 'Console'
description: 'Cobra CLI -- root command, serve, seed a create-user subcommand.'
---

# Console

## Proč

CLI je druhý vstupní bod aplikace vedle HTTP serveru. Cobra umožňuje snadno přidávat další příkazy (migrace, seedy, one-off skripty) bez změny serveru.

## Jak

Balíček `presentation/console/`. Root command `app` s podpříkazy:

```
app [command]

Available Commands:
  serve         Start the HTTP server
  seed          Seed the database with default data
  create-user   Create a user (any role -- defaults to admin)
  help          Help about any command
```

Před každým subcommandem se volá `Application.Run()`, který nejprve aplikuje pending Goose migrace -- migrace tedy doběhnou i při `seed`/`create-user`, ne jen při `serve`.

### Serve command

```go
// presentation/console/serve.go

type ServeCommand struct {
    server    *server.Server
    scheduler *scheduler.Scheduler
}

func (c *ServeCommand) Command() *cobra.Command {
    return &cobra.Command{
        Use:   "serve",
        Short: "Start the HTTP server",
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()

            schedulerDone := make(chan struct{})
            go func() {
                defer close(schedulerDone)
                c.scheduler.Run(ctx)
            }()

            serverErr := c.server.Start(ctx)
            <-schedulerDone
            return serverErr
        },
    }
}
```

`ServeCommand` orchestrátor: scheduler + HTTP server sdílí jeden `ctx` z `signal.NotifyContext`. SIGTERM drainuje obojí v tandemu. Detaily v [Scheduler](/framework/infrastructure/scheduler) a [HTTP Server](/framework/presentation/http-server).

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
        Short: "Seed the database with default data",
        RunE: func(cmd *cobra.Command, args []string) error {
            return c.seeder.Seed(cmd.Context())
        },
    }
}
```

Spuštění:

```bash
./bin/app seed   # vytvoří admin účet (heslo z APP_SEED_ADMIN_PASSWORD) pokud ještě neexistuje
```

### Create-user command

```go
// presentation/console/create_user.go

type CreateUserCommand struct {
    handler *usercmd.CreateUserHandler
}
```

Bypassuje bus (žádný auth context potřeba) a volá přímo `*usercmd.CreateUserHandler.Handle()` -- recykluje stejnou validaci, hashing a unique-nickname check jako HTTP API. V handleru `EventCollectorFromContext(ctx)` vrátí throwaway collector (bus tu nepřipravuje per-request collector), takže emitované domain eventy se tiše zahodí — pro CLI to je žádoucí.

```bash
./bin/app create-user -n alice -p secret12              # admin (default)
./bin/app create-user -n bob -p secret12 -e b@x -r user # user, s emailem
```

Flagy: `-n/--nickname` (povinné), `-p/--password` (povinné), `-e/--email` (volitelné), `-r/--role` (`admin` default, `user` alternativa).


## Detaily

- Root command (`root.go`) registruje všechny subcommandy.
- Každý příkaz dostává závislosti přes DI (Wire) a závisí na doménové interfaces (např. `shared.Seeder`), případně na application handlerech (`*usercmd.CreateUserHandler`) -- ne na konkrétní infrastruktuře.
- Komponenta `console` v `.go-arch-lint.yml` má `mayDependOn: [server, scheduler, worker, config, database, application, bus]` -- CLI volá application handlery stejně jako HTTP handlery a `serve` co-spouští scheduler + worker.
- Nové příkazy se přidávají stejným patternem: struct s `Command() *cobra.Command` metodou, registrace v root commandu.

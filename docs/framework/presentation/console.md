---
layout: 'page'
uri: '/framework/presentation/console'
position: 5
slug: 'framework-presentation-console'
parent: 'framework-presentation'
navTitle: 'CLI'
title: 'CLI'
description: 'Balíček presentation/console/ – Cobra CLI, serve command.'
---

# CLI

Balíček `presentation/console/`. Cobra CLI s příkazy.


## Příkazy

```
app [command]

Available Commands:
  serve       Spustí HTTP server
  help        Nápověda
```


## Serve command

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

```bash
./bin/app serve
# Nebo:
make serve
```

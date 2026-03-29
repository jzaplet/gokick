---
layout: 'page'
uri: '/framework/infrastructure/console'
position: 3
slug: 'framework-infrastructure-console'
parent: 'framework-infrastructure'
navTitle: 'CLI'
title: 'CLI'
description: 'Balíček console/ – Cobra CLI, serve command.'
---

# CLI

Balíček `console/`. Cobra CLI s příkazy.


## Příkazy

```
app [command]

Available Commands:
  serve       Spustí HTTP server
  help        Nápověda
```


## Serve command

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

```bash
./bin/app serve
# Nebo:
make serve
```

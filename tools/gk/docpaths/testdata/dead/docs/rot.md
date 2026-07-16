# A doc holding one of every dead-reference class

1. A path that resolves to nothing: `docs/gone.md`.

2. A cross-reference to a skill nobody kept: `/gk-missing`.

3. A line anchor past the end of the file: `docs/short.md:9999`.

4. A brace set whose second member died: `docs/{short,vanished}.md`.

5. A subtree that moved away: `tools/oldtool/**`.

6. An escape marker that exempts nothing (the path it names is never cited):

<!-- gkdoc:ignore docs/never-mentioned.md — stale escape, must be reported -->

7. An escape marker with no reason at all:

<!-- gkdoc:ignore docs/reasonless.md -->

`docs/reasonless.md`

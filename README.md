# canary

bird live in prompt. bird watch you. you code too long, bird die.

no color. no internet. no dependency. just block art + shell.

## bird

```
 ▗███▖        ▗███▖~       ▗███▖        ▗▓▓▓▖        ▗░░░▖
▐ ◉ ▌>       ▐ ^ ▌>       ▐ - ▌>       ▐ ~ ▌>       ░ x ▌v
 fresh        chirpy       tired        worn         dead
```

bird get tired from: time at shell, how many command, how long command.
night make bird more tired (22:00–07:00 → ×1.3). dead bird = go rest.

## install

```sh
curl -fsSL https://raw.githubusercontent.com/thousandflowers/canary/main/install.sh | sh
```

then open new shell. bird appear above prompt.

## tame the bird

```sh
CANARY_DISABLED=1     # bird sleep (no show)
CANARY_RESET=1        # new session, bird fresh again
CANARY_SHOW_SCORE=1   # show fatigue number 0–100
```

## fatigue math

```
score = minutes/3 + commands/2 + avg_cmd_len/10      (cap 100)
      = (min/120·40) + (count/80·40) + (avglen/200·20)
night  ×1.3

0–20 fresh   21–45 chirpy   46–70 tired   71–90 worn   91–100 dead
```

## uninstall

```sh
sh uninstall.sh
```

bird gone. rc clean. ~/.canary removed.

## shell

works zsh, bash, fish. UTF-8 terminal needed.

## license

MIT. see LICENSE.

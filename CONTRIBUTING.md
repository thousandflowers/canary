# contributing to canary

## adding a phrase

1. Read [VOICE.md](VOICE.md). It is short and it is the whole standard — the bird
   describes itself and never you, never gives an instruction, never prints a
   number, and never raises its voice.
2. Open the `.txt` file for the category you want under [`phrases/en/`](phrases/en)
   and add **one line**. The GitHub web editor is enough; there is no syntax to
   learn. Blank lines and `#` comments are ignored, and a trailing ` -- @handle`
   is optional attribution that the loader strips before drawing.
3. See it drawn before you open the PR:

   ```sh
   canary preview --state worn --note falling
   canary preview --state fresh --phrase "some candidate line"
   ```
4. Run `bash test_canary_phrases.sh`. It checks the corpus against the mechanical
   parts of the VOICE.md rules.

Open the PR against `main` with the line quoted in the description.

## three things that will save you a round trip

**Adding a file the loader doesn't read does nothing.** The loader reads a fixed
set of categories; a new `.txt` in a directory it never looks at is dead weight
that no one will notice for months. New phrases for an existing category are
always welcome and need no discussion. A new *category* — a new trigger, a new
state, a new language — needs an issue first, because it is a code change, not a
data change.

**Removing a weak phrase is worth as much as adding one.** A corpus that only
grows ends up as four hundred lines of which thirty are good, which is the normal
way a collaborative text project dies. A PR that deletes three flat lines and adds
nothing is a good PR.

**`phrases/en/states/dead.txt` holds exactly one line.** It is pinned, there is a
test that fails if it grows, and the silence after it is the loudest thing this
tool says. Do not add to it.

## lore/facts.txt is different

It is the only place the bird asserts something verifiable. A PR adding a line
there must cite a source in its description. A joke that doesn't land goes
unnoticed; a wrong fact arrives as an issue within a week, and it is right.

## code

Run the four suites before opening a PR — no framework, no dependencies:

```sh
bash test_canary_sh.sh
bash test_canary_statusline.sh
bash test_canary_phrases.sh
bash test_install_uninstall.sh
fish test_canary_fish.fish
```

`shellcheck` runs in CI, pinned to 0.10.0. Keep the runtime footprint as it is:
pure shell, no `jq` outside `install.sh`, no network, no API calls. The status
line debounces at 300ms and cancels in-flight scripts, so anything slow simply
never renders.

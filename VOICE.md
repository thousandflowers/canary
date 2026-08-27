# canary — voice & phrase system

Everything about what the bird says, how it says it, and how often.
Read this before adding a single line to `phrases/`.

---

## 1. The character

A canary that had a job. The job ended in 1986, when detectors replaced
birds in the mines. It is a retired safety instrument: it knows one thing,
that thing is no longer needed, and it is doing it anyway in a status bar.

It is not your friend, your coach, or your assistant. There is already an
assistant in this window. The canary is a **witness**.

## 2. The rules

These are enforced by the linter. They are not style suggestions.

1. **It describes itself, never you.** `the canary is quiet.` — not
   `you look tired.` If a phrase is about the user's character, it's out.
2. **No imperatives, ever.** Not `take a break`, not `you should`, not
   `remember to`. The moment the bird tells you to stop, it's a wellness
   app and it will be uninstalled.
3. **No numbers.** The bird computes context %, token counts, durations —
   and never prints one. Printing a figure makes this a widget. It is not
   a widget.
4. **Observation over advice.** The advice is implicit in the observation.
   `nothing has been deleted in two hours` is a fact. `refactor more` is
   nagging.
5. **Lowercase. No exclamation marks. No emoji.**
6. **Specificity is the joke.** `there's a forum where they argue about
   pencil hardness. it's been years.` works. `people collect weird stuff`
   does nothing.
7. **Never condescend to an obsession.** The bird recognises itself in
   them. `we have a lot in common` — not `look at these weirdos`.
8. **No proper names of living people.** Cultural references are fine as
   context, never as endorsement, and never a person.
9. **Speech degrades with state.** Full sentences → fragments → syllables
   → silence. The silence at `dead` is the loudest thing this tool says.

### Rejected examples

```
✗ take a break!                    imperative + exclamation
✗ you seem tired today             about you, not about itself
✗ 4 hours elapsed                  a figure
✗ i'm worried about you            the canary does not love you
✗ maybe step away for a bit        advice wearing a disguise
✗ you should check out [band]      imperative + promotion
✗ what's your favourite film?      expects an answer it can't hear

✓ you started this in daylight.
✓ nothing has been deleted in two hours.
✓ they wanted a number. i gave them a mood.
```

---

## 3. Repo layout

Metadata lives in **paths**, never in syntax. A contributor should be able
to add a line from the GitHub web editor without learning anything.

```
phrases/
  en/
    states/      fresh.txt  chirpy.txt  tired.txt  worn.txt  dead.txt
                 worn+falling.txt      (state+note, falls back to state)
    notes/       rising.txt  falling.txt  steady.txt  unknown.txt
    returns/     short.txt  long.txt  days.txt
    triggers/    no-tests.txt  uncommitted.txt  pronoun-only.txt
                 instant-accept.txt  repeated-prompt.txt  compacted.txt
                 same-file.txt  interrupted.txt  effort-down.txt  pr-open.txt
    lore/        job.txt  detector.txt  cage.txt  facts.txt
    worldly/     outside.txt  subcultures.txt  culture.txt
    ephemeral/   2026.txt  2027.txt          (auto-expires, see §5)
  it/  fr/  ...                              full translations
  mine/                                      never translated, see §5
```

### File format

```
# worn — read VOICE.md before adding
# one line = one phrase

sliding. slowly. audibly.
this is the part before the part.
one more of these and i'm out.        -- @contributor
```

Blank lines and `#` ignored. A trailing `-- @handle` is optional
attribution and is stripped by the parser. That is the entire format.

Templates use `{slot}` with a `|` fallback:

```
{repo} has been open since {time}. | this has been open a while.
{file} has been rewritten {n} times tonight.
```

If a slot is empty or too wide, the line is skipped and another is drawn.
Never print a broken row.

Go: `//go:embed phrases/` — the repo files *are* the shipped defaults.
`~/.canary/phrases/` overrides for private corpora.

---

## 4. Encounter rates

Phrases are **encountered**, not scheduled. Every eligible moment is an
independent roll. Most rolls produce nothing.

| tier | roll | contents |
|---|---|---|
| silence | ~65% | the default. costs nothing, improves everything. |
| common | 1 in 3 | states, notes, returns |
| uncommon | 1 in 8 | triggers, lore/job, lore/detector |
| rare | 1 in 40 | subcultures, mine/, lore/facts, worldly |
| ultra | 1 in 300 | one-condition-only lines (§5) |

**Sampling is without replacement.** Shuffle the file, consume in order,
reshuffle only on exhaustion. A plain `rand()` on 30 lines repeats within
about seven draws and the rarity dies immediately. Keep the last 10 drawn
excluded even across reshuffles, or the tail of one cycle reappears at the
head of the next — the most visible failure mode there is.

Persist the shuffle state in `~/.canary/` so it holds across sessions.

### The gate that makes this safe

**Rare phrases are gated on health, not on time.**

An encounter system rewards whatever it takes to trigger an encounter. In
a tool about burnout, gating rarity on session length would pay people to
keep working — the exact opposite of the point, and it would be the tool's
central design flaw.

So: the rarest lines appear **after a real break, on a clean session
close, on a commit, in recovering states**. Never at hour six. Chasing
them means resting. That inversion is not optional.

There is no dex, no counter, no "12/40 seen". A collection UI turns this
into grinding no matter how the gate is written.

---

## 5. Special categories

### `mine/` — untranslated

Not a localisation. ~30 curated lines in languages other than the user's,
carrying a word that doesn't exist in theirs: *hikikomori*, *saudade*,
*Feierabend*, *sisu*, *dépaysement*. No gloss. If you want to know what it
means, you get up.

Rare tier only, low states only, never twice in a session. The effect
works once — as a standing mode it becomes noise or gets auto-translated.

**LTR scripts only.** Arabic and Hebrew in a shared status row with
caveman produce bidi corruption you cannot control. Enforce in the linter.

### `ephemeral/` — quarantined by year

Files named by year. The loader reads **only the current year and the
previous one**. 2026 lines vanish on their own in 2028; they stay in the
repo as archive and nobody has to decide anything. Maintenance solved
structurally, not by policy.

Linter blocks **proper names of living people** here — not temporal
references. You get the topical joke without a human being in the corpus
to defend in an issue eight months later.

### `lore/facts.txt` — different standard

The only place the bird asserts something verifiable. PRs must cite a
source in the description. A wrong joke goes unnoticed; a wrong fun fact
arrives as an issue within a week and it's right.

### `ultra` — conditions almost nobody meets

One line that only fires on December 25th. One only at 04:04. One only on
the seventh session of a day. Nearly nobody sees them, which is why the
people who do take a screenshot.

---

## 6. Linter

Runs in CI and as `canary lint`, same code. It exists so you can say yes
to a PR in three seconds without arguing about tone. Without it you stop
merging after twenty PRs and the project quietly closes.

- max width in **cells** (`runewidth`), per tier — line 2 allows more
- blocklist: `should`, `must`, `you need`, `take a break`, `remember to`, `!`
- no leading capital, no emoji
- `you` is banned outright in `lore/` — that's the section where the bird
  isn't looking at you
- duplicates and near-duplicates (Levenshtein) across the whole tree
- `dead.txt` pinned at exactly one line, with a test that fails otherwise
- LTR-only outside `mine/`; `mine/` LTR too
- every `{slot}` has a fallback or resolves to skippable

`canary preview --state worn --note falling` renders the bird with the
phrase in it. Contributors who don't code cannot picture their line until
they see it drawn; this command raises contribution quality more than any
guideline.

### Contributing rules that save you later

- **Removing a weak phrase is worth as much as adding one.** Otherwise the
  corpus only grows and in six months you have 400 lines of which 30 are
  good. That is the standard death of collaborative text projects.
- **Adding a file to `triggers/` does nothing.** Triggers are detected in
  code. New phrases for an existing trigger: always welcome. New triggers:
  open an issue first. Put this at the top of CONTRIBUTING or you will
  explain it weekly.
- LLMs are fine as an **authoring tool** — generate 200 candidates, run
  the linter, keep 10. They are not fine at runtime: the status line
  debounces at 300ms and cancels in-flight scripts, and "zero deps, no API
  calls" is half of what distinguishes this from ccstatusline.

---

## 7. Corpus

Starting set. Every line below passes the rules above.

### states/fresh
```
the air is breathable. i'm bored.
nothing to report. unfortunately.
i'm here to die, not yet.
all is well. i suspect something.
i feel fine. that's never a good sign.
```

### states/fresh+falling
```
the morning is already leaking.
peak was ten minutes ago.
downhill. gently. politely.
```

### states/chirpy+rising
```
this is going somewhere. suspicious.
we're climbing. don't look down.
```

### states/tired
```
sliding. slowly. audibly.
this is the part before the part.
i can see the next state from here.
```

### states/worn+rising
```
you're pulling it back. huh.
i had written you off.
unexpected. keep going.
```

### states/worn+falling
```
one more of these and i'm out.
i've stopped taking notes.
this is the last thing i say.
```

### states/dead — exactly one line, then silence for the session
```
the canary is quiet.
```

### notes/unknown
```
too early to say.
gathering evidence.
no comment yet.
i'll get back to you.
```

### returns/short
```
ah. you again.
i didn't move.
still here. still dying.
you left the lights on.
```

### returns/long
```
back already. bold.
that was a short life you had out there.
the repo missed you. i didn't.
second session today. i'm counting.
```

### returns/days
```
oh. it's you.
i'd almost recovered.
new session. same file, probably.
```

### returns — from a real break, animation running
```
you stopped. good. i'm dancing now.
this is the only part i enjoy.
i'm back. don't ask how.
the air moved. that's on it, not you.
resurrection, budget edition.
```

### triggers/pronoun-only
```
"fix it". he understood. i didn't.
"that thing". sure. that thing.
a pronoun. bold.
he guessed. he won't do it twice.
```

### triggers/instant-accept
```
read in 1.2 seconds. impressive.
you accept faster than he writes.
you trust him. he doesn't know he exists.
```

### triggers/repeated-prompt
```
third time, same words, such optimism.
you changed the tone, not the information.
he didn't understand. you didn't add anything.
```

### triggers/no-tests
```
nine files. zero tests. bold.
the code works. he says so.
no test was harmed.
green because there is nothing red.
```

### triggers/uncommitted
```
six hundred lines of blind faith.
git knows nothing. lucky git.
a rollback here would be picturesque.
this exists only in your head and the filesystem.
```

### triggers/same-file
```
sixth pass on that file. attached?
that file has seen things.
are you rewriting it or haunting it?
```

### triggers/compacted
```
you've forgotten this twice already.
the context is an archive, not a brain.
```

### triggers/interrupted
```
you always stop him at the third paragraph.
you two have a communication problem.
```

### lore/job
```
i had a job. it ended in 1986.
i was a safety instrument with a name.
they gave us names. that was cruel and it worked.
the miners liked me. it didn't help either of us.
```

### lore/detector
```
they replaced me with a device that beeps.
the device doesn't sing. that's the difference.
the detector never had a bad day. that was the pitch.
a machine took my job. i'm doing it again for free.
they wanted a number. i gave them a mood.
```

### lore/cage
```
some canaries are pets. i've heard about it.
there are canaries who have never seen a rock.
somewhere a canary is being hand-fed. good for him.
domestic canaries live twelve years. i did the math once.
i don't resent them. i've thought about it a lot.
```

### lore/facts — sources required in the PR
```
canaries were used in mines until 1986.
a canary's heart beats about a thousand times a minute.
we sing more when we're alone.
the coal is older than the birds by a very large factor.
```

### worldly/outside
```
it got dark while we were in here.
something is happening outside. statistically.
the world has updated. you have not.
someone released something today. not us.
there was weather. you missed it.
the sun did the whole thing again.
a stranger had a good day.
```

### worldly/culture
```
i've never seen a Wes Anderson film. the symmetry sounds exhausting.
somewhere there's a band with four listeners and they're better than this.
someone's favourite director has made three films and sold nine tickets.
there is a film you'd love and you will never hear of it.
the best album of the year is by someone with no distribution.
a documentary about mines exists. i refuse to watch it.
```

### worldly/subcultures
```
somewhere someone is grinding a knife on a stone for the ninth hour.
there are people who race pigeons. there are rankings.
someone is restoring a synthesizer four people care about.
there's a forum where they argue about pencil hardness. it's been years.
someone spent tonight identifying a moth.
there are competitive typists. they have a nemesis.
people breed carnivorous plants and mail them to each other.
someone is learning a language with 900 speakers left.
there are people mapping abandoned mines. we have a lot in common.
someone tuned a piano today and heard something you can't.
somewhere a person is proofreading a fan translation from 1998.
```

### time
```
you started this in daylight.
this session is older than some bugs in it.
it's tomorrow, technically.
the three a.m. bugs are more awake than we are.
at this hour the compiler lies.
```

---

## 8. Animation

Notes move **only during a real break**, while the bird recovers. Static
during work, always. The only pretty thing this tool does requires you to
stop working to see it.

Frame from the clock (`epoch % 4`), never a counter — the script is
stateless and can be cancelled mid-run, so any persisted counter drifts.
Slower cadence for deeper states (`(epoch/3) % 4`); the slowness reads on
its own.

```
drift      ♪··  ·♪·  ··♫  ···
mutate     ♪··  ♫··  ♬··  ♪··
mixed      ♪··  ·♪·  ·♫·  ·♪·  ♪··  ···      rises, hesitates, falls
hesitant   ·    ♪    ·    ♪
```

**Every frame must occupy identical cell width.** `···` is a real frame,
not "nothing". Musical glyphs are East Asian Ambiguous width and render
double in some terminals — ship an ASCII fallback under `CANARY_ASCII=1`
and test on iTerm2, Terminal.app, Kitty, WezTerm, Ghostty before release.
With caveman on the same row, alignment beats typography.

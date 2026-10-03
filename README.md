<div align="center">

# Tutor

**Build Magic: The Gathering decks from your terminal.**

A fast, keyboard-driven TUI that puts Scryfall search, your Moxfield decks, card rulings, and the comprehensive rules side by side — so you can go from "what removal is in these colours?" to a tagged, legal decklist without ever touching a mouse.

![License: MIT](https://img.shields.io/badge/license-MIT-green)
![Platform: Linux · macOS · Windows](https://img.shields.io/badge/platform-Linux%20%C2%B7%20macOS%20%C2%B7%20Windows-blue)
![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)

![The Tutor workspace: a Scryfall search, the deck being built, and the card info panel side by side](screenshots/hero.png)

</div>

## What is this?

**Tutor** is a deck-building tool for Magic: The Gathering — `ttr` on the command line. It runs entirely in the terminal and is driven by the keyboard.

I search for cards on Scryfall and build decks on Moxfield, and juggling two web apps in two browser tabs had accumulated a pile of small frictions. Tutor is my attempt to file them all off at once: the search results and the decklist live in the same window, the rulings sit right next to the card, and adding a card to a deck is a single keystroke.

## Why not just use Scryfall and Moxfield?

You still can — Tutor reads from both. It just fixes the parts of that workflow that used to slow me down:

- **Search and decklist, side by side.** Any number of panels in one window: several Scryfall searches, several decklists, the rules — all open at once, all keyboard-navigable.
- **Add cards with a keystroke.** Scroll a search, press `a`, and the card lands in the deck you're editing. `x` takes it back out.
- **The whole card, where you're looking.** Oracle text, rulings and legalities sit in the panel beside the list, not below a wall of buttons. `gx` swaps in the card's picture with its price, `H`/`L` page through every artwork it has had, and `f` turns a double-faced card over.
- **Dense lists.** Rows instead of tiles, so you see more of a search at once. The art is a key away when you want it, not in the way when you don't.
- **Tags that do the work.** Tag a whole theme in one stroke, or tag a deck by Scryfall's oracle tags. Keep tag lists whose tags count in every deck, and move tags from one list to another. Then filter, sort and count by them.
- **Statistics you can drill into.** Filter a list by tag or category and watch the histograms recompute for exactly that subset, or ask the odds of each one turning up in your opening hand.
- **Every change kept, on every machine.** Every deck is a plain file in a git repository, so nothing you change is lost and `git log` works on your decks. Point Tutor at a private remote you own and `space s` keeps your whole collection in step everywhere.
- **Moxfield built in.** Follow a deck by URL, or browse someone's decks by username, and pull a copy in to edit.
- **Quick answers from the shell.** `ttr <card name>` prints the card — text, rulings, legality, price — and gets out of the way.
- **A couple of extras I find handy:** the comprehensive rules are searchable and drive keyword highlighting in card text, and you can see how a card's wording has changed across printings.

## A quick tour

### Rulings, right next to the card

Scroll a search and the panel on the right fills in with the highlighted card: oracle text with keywords picked out, every ruling, and its format legalities. No scrolling past buttons to find them.

![A Scryfall search with the info panel showing keyword-highlighted oracle text, rulings, and legalities](screenshots/rulings-inline.png)

### Deck on the left, search on the right

Open a search beside the deck you're building. Cards already in the deck are marked, so you can see your coverage as you scan. Each mark takes the colour of the border of the panel the card is also in, and a shape of its own: an aqua ● for the deck you're editing, an orange ◆ for the panel you're in, a grey ▲ for any other list. The deck wins when a card is in several. `a` adds the highlighted card, `x` removes a copy, `u` undoes.

![A search panel beside the editing deck, with cards marked as already in the deck](screenshots/add-to-deck.png)

### Two sorts, either way round

Every list has two sorts. `.` cycles the first, which fills the column on the right: mana value, colour, type, power, toughness, EDHREC rank, price, rarity. `,` cycles the second, which breaks the first one's ties and colours the names on the left. Sorted by mana value and then by type, the curve reads from the top down, the creatures at each cost grouped together, their names coloured by type. `alt+.` and `alt+,` turn either one round. Each starts the way it reads best: power and price from the top, EDHREC rank from 1. The sorts stay put when you run a new search.

The *inclusion* sort orders a list by where else its cards are: first the ones already in the deck you're building, then the ones in another list on screen, then the ones found only here. Add a card and it moves up to join the others, and the next card slides under the cursor. Which sorts `.` and `,` cycle through, and in what order, is yours to set in `config.json`.

![A search sorted by mana value and then by type, names coloured by type](screenshots/sorts.png)

### Tag a whole theme at once

Narrow a list with `/`, select what's left with `V`, and tag them together with `t`. `A` adds them to the deck and tags them in one stroke — sorting a search into a deck is dozens of these.

**Tag lists.** Any `.list` file can hold tags for other lists to use — `f-edh-id-rbg-otag-removal.list`, say, with a line per card and its tags, in the same format as a deck, so it uploads to Moxfield like one. Press `t` on it in the decks panel and its tags count in every list: a deck where you've tagged nothing as removal still shows removal in its statistics, and `/` and the statistics filter find it. The tags aren't written into the deck. The tag lists that are on show under a `tag lists` folder at the top of the decks panel.

**Moving tags between lists.** `T` moves the tags of a whole list at once. It uses the cards you picked with `v`, or else every card showing, so filter first to move only some:

| Keys | Does |
| --- | --- |
| `T t` | this list's tags onto the editing deck, for the cards it already has |
| `T a` | the same, and the cards the editing deck lacks are added with their tags — for growing a tag list |
| `T g` | the tag lists' tags, written into this list's own |
| `T m` | every list on screen gets the others' tags, for the cards it has (only your own lists change) |

![Selecting several cards in a search and tagging them in bulk](screenshots/tagging.png)

### Statistics that answer questions

Open the statistics panel and walk the breakdown — tags, types, colours, the curve. Tags, types and colours lead with the commonest; on a tag, `tab` sorts the tags by name instead, which keeps families like `otag-…` together. Build a filter from the categories with AND (`a`), OR (`o`) and NOT (`n`) — *ramp and not lands*, *removal or counterspells* — and the list and the histograms narrow to that subset as you go. Moxfield shows you stats; here you can interrogate them.

![The statistics panel: histograms of the deck, filtered to a combination of categories](screenshots/stats-filter.png)

`p` swaps the counts for the odds of drawing them: the chance that your opening seven holds at least one, two, three or four cards from each category — hypergeometric, worked out over the deck as it stands. `p` again steps from at least one up to at least four. How often is there ramp in your opener? At least three lands? Each bar says.

![Opening-hand odds: the chance of at least one card of each tag, type and colour in the first seven](screenshots/odds.png)

### The card as printed

Press `gx` on a card to see it as printed, in the info panel: the printing your list holds, which is the one Scryfall shows unless you pinned another. Under the picture are the card's tags, its price in that printing, where it's legal and its rulings — `K`/`J` scroll down to them. `H` and `L` step to older and newer artworks, and `f` shows the other face of a double-faced card. The picture follows the cursor as you move, and `gX` fetches every card's picture in the list ahead of you, so walking it is instant. Tutor comes back to this view if you quit in it. In kitty, Ghostty and WezTerm it's drawn right in the terminal; anywhere else, `gx` opens it in your browser. `gx` on a Moxfield deck in the decks panel opens it on Moxfield.

![A search with the highlighted card's printing drawn in the info panel](screenshots/printing.png)

### The rules, searchable

Search the comprehensive rules and read the full paragraph in the info panel as you scroll. Open the rules while a card is highlighted and you get the rules that card actually invokes.

![The rules browser: the rules a highlighted card invokes on the left, the full rule on the right](screenshots/rules.png)

### How a card's text has changed

Press `gv` on a card to see its printed wording across every printing — the errata and templating changes, side by side.

![A card's oracle text diffed across its printings](screenshots/text-history.png)

### Your decks, and Moxfield's

The decks panel lists your local decks alongside the Moxfield decks and users you follow, with colour identity, card counts, legality, and age. Follow a deck by pasting its URL; follow a person by typing their username, and their public decks drop down under them like a folder.

![The decks panel listing local and followed Moxfield decks with colours and legality flags](screenshots/decks.png)

### Every change, kept

Each deck is a file in a git repository, so Tutor keeps every version. Press `gv` on a deck to walk its history and read the diff for each change — what you added, what you cut, and when.

![A deck's version history, with the diff for the selected version shown in the info panel](screenshots/deck-git.png)

And because it's a git repository, it can sync. Point Tutor at a private remote you own — `ttr sync remote <url>` — and `space s` from anywhere (or `ttr sync`) mirrors your whole collection to it and pulls back whatever you changed on another machine. It's plain git, so any host works — GitHub, Codeberg, GitLab, or your own server — and there's nothing to install beyond the git you already have. A remote you seeded with a README merges in cleanly on the first sync; the one thing git can't decide for you — the same deck edited two places at once — surfaces as a conflict to resolve with git, never a silent overwrite. Make the repo **private**; your decks are yours.

## Install

Tutor is a single self-contained binary, `ttr`. Grab a prebuilt one, or build from source.

### Prebuilt binaries

Download the right file for your machine from the [latest release](https://github.com/marbris/tutor/releases):

| Platform | File |
| --- | --- |
| Linux (x86-64) | `ttr-linux-amd64` |
| Linux (ARM64) | `ttr-linux-arm64` |
| macOS (Apple Silicon) | `ttr-darwin-arm64` |
| macOS (Intel) | `ttr-darwin-amd64` |
| Windows (x86-64) | `ttr-windows-amd64.exe` |

**Linux**

```bash
chmod +x ttr-linux-amd64
mv ttr-linux-amd64 ~/.local/bin/ttr   # make sure ~/.local/bin is on your PATH
```

**macOS**

```bash
chmod +x ttr-darwin-arm64
xattr -d com.apple.quarantine ttr-darwin-arm64   # clears the "unidentified developer" block
mv ttr-darwin-arm64 /usr/local/bin/ttr
```

**Windows**

Download `ttr-windows-amd64.exe`, rename it to `ttr.exe`, and put it somewhere on your `PATH`.

### From source

Requires [Go 1.26+](https://go.dev/dl/).

```bash
git clone https://github.com/marbris/tutor.git
cd tutor
go build -o ttr .

# Then put it on your PATH — a symlink means rebuilds are picked up automatically:
ln -s "$(pwd)/ttr" ~/.local/bin/ttr
# ...or just move it:  mv ttr /usr/local/bin/
```

To build binaries for every platform at once (they land in `dist/`):

```bash
make all
```

## First run

Just run it:

```bash
ttr
```

You'll land on a splash screen. Everything is discoverable from three keys:

- **`space`** opens the menu of things you can do (create panels, close them, commit).
- **`?`** grows the hint bar at the bottom to the full list of keys for wherever you are. `?` again shrinks it back.
- **`q`** quits.

Press `space` then `f` to open your first Scryfall search.

## How it works

The screen is a **row of panels** with an **information panel** pinned to the right. You can open as many panels as you like — searches, decklists, the rules — and move between them with `h`/`l` (or the arrow keys). The info panel always describes whatever is highlighted in the focused panel.

Open panels straight to what you want:

| Keys | Opens |
| --- | --- |
| `space f` | a **Scryfall search** |
| `space d` | your **decks** (and Moxfield) |
| `space r` | the **comprehensive rules** |
| `space n` | a blank panel — `tab` cycles the search target |

**The editing deck.** One local deck is marked as the one you're editing — it carries a distinct border colour. The card-moving keys (`a` add, `x` remove, `t`/`T` tag, `c` commander) always act on *that* deck, from whatever panel you're in, so you can add to it from a search two panels over. With one deck open it's chosen automatically; `e`/`E` pick another; `gd` jumps to it.

**Building a deck, start to finish:**

1. `space d` → `n` to make a new deck (or open an existing one). It becomes the editing deck.
2. `space f` and run a Scryfall query.
3. Scroll the results; the info panel shows each card. Press `a` to add the highlighted card, or select several with `v`/`V` and add them together.
4. `/` filters what's on screen, `.`/`>` re-sorts it (mana value, colour, type, power/toughness, EDHREC rank…) and `,`/`<` sorts within that, `alt+.`/`alt+,` turn either round, `t` tags the selection.
5. `s` opens statistics for the list; walk the categories with `j`/`k` and add them to the filter with `a`/`o`.
6. Every edit is written to the deck file as you make it; `w` commits it to git. `space w` commits every open deck from anywhere, and `space s` pushes them to your git remote.

## Command line

`ttr` is useful without opening the interface at all:

```bash
ttr                                   # come back to the panels you left, sorted and filtered as they were
ttr 't:creature c:rw cmc<=3'          # run a Scryfall query
ttr Isshin                            # one exact match prints straight to the terminal
ttr https://moxfield.com/decks/...    # open a deck on Moxfield
```

![ttr printing a single card's details to stdout, no interface](screenshots/quick-lookup.png)

Queries use [Scryfall's own syntax](https://scryfall.com/docs/syntax), so anything that works on the website works here.

There are a few subcommands, each with its own `-h`:

```bash
ttr deck list                   # your decks
ttr deck new <name> [format]    # start an empty deck
ttr deck import <id|url> [as]   # copy a Moxfield deck in so you can edit it
ttr deck log <name>             # what you've changed, and when (it's git)
ttr deck restore <name> <ref>   # bring back an earlier version
ttr deck dir                    # where your decks live on disk

ttr sync                        # push and pull your decks
ttr sync remote <url>           # connect a private git remote you own
ttr sync status                 # what's ahead or behind
ttr sync off                    # disconnect (your decks are untouched)

ttr rules <query>               # search the comprehensive rules
ttr theme                       # list colour themes
ttr theme <name>                # switch theme
ttr keys                        # list every key binding
ttr keys --defaults             # print the defaults as a keys.json to edit
ttr init                        # write commented-out templates of every settings file
ttr cache                       # what's downloaded, and how much room it takes
ttr cache clear [kind]          # empty it — all of it, or pictures, texts, rules…
```

Because every deck is a file in a git repository, `git log`, `git diff`, and friends work on your decks directly.

## Key bindings

The bottom of the screen shows `?` and `q`. Press `?` and each panel shows its own keys along its bottom, the deck you're editing shows the keys that change it, and the bottom of the screen shows the `space` menu. The essentials:

<details>
<summary><b>Full key reference</b></summary>

**Getting around**

| Key | Does |
| --- | --- |
| `h` `l` / `←` `→` | previous / next panel |
| `ctrl+h` `ctrl+l` | move the focused panel along the row |
| `j` `k` | up / down in the list |
| `gg` `G` | first / last row |
| `K` `J` | scroll the info panel half a screen |
| `H` `L` | in the printing view: an older / newer artwork |
| `f` | in the printing view: the other face of a double-faced card |
| `b` `B` | clear this list's filters · clear every list's filters |
| `space` | the menu · `?` show keys in the panels · `q` quit |

**In a list of cards**

| Key | Does |
| --- | --- |
| `/` | filter as you type |
| `.` `>` | cycle the sort order, which fills the right-hand column |
| `,` `<` | cycle the second sort order, which colours the names on the left |
| `alt+.` `alt+,` | turn the first / second sort round (ascending ↑, descending ↓) |
| `i` | edit the search · on a deck of yours, add a card from Scryfall — `tab` there tags the deck by oracle tag instead: `removal` tags every card Scryfall calls `otag:removal` with `otag-removal` |
| `v` `V` | select one / all shown |
| `a` `A` | add a copy to the editing deck · add-and-tag with the last tag |
| `t` `x` | tag the selection (`tab` completes a tag you already use) · remove a copy |
| `T` then `t` `a` `g` `m` | move tags a whole list at a time — see *Tag a whole theme at once* |
| `c` | set as the editing deck's commander |
| `u` | undo the last edit |
| `y` `p` | yank the selection · put it into this list |
| `s` `S` | statistics for this list · for the editing deck |
| `gv` | how this card's printed text has changed |
| `gx` | the card as printed, in the info panel (or the browser) |
| `gX` | the same, and fetch every card's picture in the list ahead of you |
| `w` `W` | commit this deck · save a search or remote deck as a deck of yours (here / in a new panel) |

**In the decks panel**

The panel is a folder tree: local decks group by the folder part of their slug
(`aggro/mono-red`), and the Moxfield decks and people you follow sit under one
`moxfield` folder, listed first. Folders start collapsed; `enter` opens one. A
person you follow is a folder of their public decks, cached so the `/` filter
finds them without opening anything.

A deck's name is just its name — the folder is where its file lives, not part of
the name. Renaming a deck changes its name; renaming it to `dirname/deckname`
also moves it into `dirname` (made if it isn't there, beside the deck; a leading
`/` starts from the top). `r` on a folder renames the folder and takes its decks
with it. Move a deck between folders with cut and put
(`x` then `p`). Naming a new deck into a folder (`aggro/Mono Red`) files it there
— the folder goes to the location, and the deck is named `Mono Red`. Ending the
name with a slash (`aggro/`) makes an empty folder to fill later. A deck copied
from Moxfield never lands in a folder: a slash in its title becomes a space.

| Key | Does |
| --- | --- |
| `enter` | fold/unfold a folder or person, or open the deck |
| `L` | open beside, in a new panel |
| `i` | follow a Moxfield deck URL or a username |
| `n` `r` | new (in the current folder) · rename |
| `c` `C` | copy deck · copy deck & its considering list |
| `y` `x` `p` | yank (copy) · cut (move) · put into the folder you're on |
| `d` | delete |
| `t` | use this list as a tag list, or stop |
| `gv` | git versions of the deck |
| `gx` | open a Moxfield deck on moxfield.com |

**The editing deck**

| Key | Does |
| --- | --- |
| `e` `E` | choose which deck to edit |
| `gd` | jump to the editing deck |
| `space w` | commit every open deck with uncommitted edits, from anywhere |
| `space s` | git push: sync your decks with their git remote |

**Panels**

| Key | Does |
| --- | --- |
| `space f` `space d` `space r` | new search / decks / rules panel |
| `space n` | new blank panel (`tab` picks its target) |
| `space c` `space o` | close this panel / close the others |
| `space u` | bring back the panel you last closed |

</details>

### Rebinding keys

Every key is a default you can move. Put the ones you want changed in `~/.config/ttr/keys.json`, grouped by where the key acts. To put the sorts back on the keys they used to have, and bring back `,` as a second leader:

```json
{
  "cards":  { "sort1.next": ["o"], "sort1.prev": ["O"],
              "sort2.next": ["'"], "sort2.prev": ["\""] },
  "global": { "leader": ["space", ","] }
}
```

`ttr keys` lists every scope, action and key as they're bound now, and `ttr keys --defaults` prints the whole keymap as a file to start from — or run `ttr init`, which writes it into place with every line commented out. The hints on screen follow your bindings. A clash within one scope is reported on startup, and that scope keeps its defaults until it's fixed. `ttr keys -h` has the details.

### Sort orders

Which sorts `.` and `,` step through, in what order, and which way each starts, live in `~/.config/ttr/config.json`. Leave a sort out of the cycle and it isn't offered. `name` is left out as it ships:

```json
{
  "sort": {
    "cycle": ["scryfall", "mana value", "colour", "type", "power", "toughness",
              "edhrec", "usd", "rarity", "inclusion"],
    "direction": { "power": "desc", "usd": "desc", "rarity": "desc" }
  }
}
```

`ttr init` writes every settings file — `config.json`, `keys.json` and a theme to start from — with all the defaults in them, commented out: they change nothing until you uncomment a line, and they show you everything there is to change. Settings files may carry `//` comments.

## Where your files live, and how to uninstall

*Tutor used to be called scry. If you used it under that name, your decks, settings and downloads move to the new directories below the first time `ttr` runs. Nothing is left behind, and nothing needs doing.*

Tutor follows the standard per-user directories for your OS. Four kinds of file live in four places, which is what tells a backup what to keep and an uninstall what's safe to delete:

| | Holds | Linux | macOS | Windows |
| --- | --- | --- | --- | --- |
| **Data** | your decks (back this up!) | `~/.local/share/ttr` | `~/Library/Application Support/ttr` | `%AppData%\ttr` |
| **Config** | settings, keys and themes | `~/.config/ttr` | `~/Library/Application Support/ttr` | `%AppData%\ttr` |
| **State** | session, query history | `~/.local/state/ttr` | `~/Library/Application Support/ttr` | `%AppData%\ttr` |
| **Cache** | downloaded card data, rules | `~/.cache/ttr` | `~/Library/Caches/ttr` | `%LocalAppData%\ttr` |

Each deck is a plain-text `.list` file, and a tag list (below) is one too. Files from before 5.0 were called `.deck`; the first run renames them in one git commit, history and all.

`ttr deck dir` prints your decks directory. Set `TTR_DECKS_DIR` to keep them somewhere else.

**To uninstall:** delete the binary, then remove the directories above. On Linux:

```bash
rm ~/.local/bin/ttr                       # the binary (wherever you put it)
rm -rf ~/.local/share/ttr ~/.config/ttr ~/.local/state/ttr ~/.cache/ttr
```

⚠️ The **data** directory is your decks. Copy it somewhere first if you want to keep them.

## Credits and sources

Tutor is a client for other people's excellent, freely available data. It wouldn't exist without:

- **[Scryfall](https://scryfall.com/)** — card data and the search syntax, via their [free API](https://scryfall.com/docs/api).
- **[Moxfield](https://moxfield.com/)** — public decklists and user decks.
- **[MTGJSON](https://mtgjson.com/)** — the printed text of every printing, for the wording history.
- **Wizards of the Coast** — the [comprehensive rules](https://magic.wizards.com/en/rules), which power the rules browser and the keyword highlighting.
- **[EDHREC](https://edhrec.com/)** — Commander play-rate rankings (the default search order).
- **The [Scryfall Tagger](https://tagger.scryfall.com/) community** — the oracle tags behind `otag:` searches.

Built with the **[Charm](https://charm.sh/)** stack — [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## License

MIT — see [LICENSE](LICENSE).

---

*Magic: The Gathering is © Wizards of the Coast. Tutor is an unofficial fan-made tool, not produced by, endorsed by, or affiliated with Wizards of the Coast. All card names, rules text, and related content are property of their respective owners.*

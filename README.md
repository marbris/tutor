<div align="center">

# Tutor

**Build Magic: The Gathering decks in the terminal.**

![License: MIT](https://img.shields.io/badge/license-MIT-green)
![Platform: Linux · macOS · Windows](https://img.shields.io/badge/platform-Linux%20%C2%B7%20macOS%20%C2%B7%20Windows-blue)
![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)

![A Scryfall search, the deck being built, and the card info panel side by side](screenshots/hero.png)

</div>

## What is this?

Tutor is a deck-building tool for Magic: The Gathering. It runs in the terminal, it's driven by the
keyboard, and the command is `ttr`.

I search for cards on Scryfall and build decks on Moxfield. I'm grateful to the people who make them.
But working between two web interfaces is a headache, so I made my own deck-building app. Here are my
favorite things about it:

- Keyboard only. Scryfall searches and decks sit side by side, and a card goes into the deck with `a`.
- Every deck is a plain text file in git. Every version is kept, and you can sync them to any git host.
- You can see the Scryfall Tagger oracle tags of every card, and count your deck by them.
- Tagging in bulk, and tag lists that count in every deck.
- Filtering a deck by combinations of tags and properties: AND, OR, NOT.
- Your own theme colors.

## Why I made it

Scryfall, EDHREC, Moxfield and Scryfall Tagger are pillars of the Magic community. Tutor wouldn't be
possible without them. This is how I used them before.

I go to Scryfall and search for a commander in the general area. I want to build a Jund aristocrats
deck where power matters:

```
f:edh is:commander id=rbg o:power
```

I look through the cards and pick Ziatora, the Incinerator. I go to Moxfield and create the deck with
the commander. Then I start copying card names from Scryfall into Moxfield, jumping between Scryfall
queries in different browser tabs:

```
f:edh id:rbg o:power otag:removal
f:edh id:rbg o:power otag:protection
f:edh id:rbg o:power otag:ramp
f:edh id:rbg o:"sacrifice this at end of turn"
f:edh id:rbg o:copy
f:edh id:rbg t:creature order:power direction:desc
```

Somewhere in there I'm googling rules and reading the MTG wiki and Reddit.

At around 120 non-land cards in the main deck and 150 in considering, I start tagging: ramp, removal,
protection, etb, etb-has, etb-likes, etb-gives, flying-has, flying-gives, sac-eot, sac-outlet.

Days and weeks go by, adding cards and tagging. Now there are 150 non-land cards in the main deck and
200 in considering. I have no idea how many cards are in both. I have no idea how many of the cards
with flying are tagged as having flying. It's a mess.

Time to start from scratch. I make a new deck on Moxfield, copy over all the cards and tags, put
everything in considering, and move them back one card at a time with a grand strategy in mind. I need:

```
10 removal
10 ramp
 5 board wipes
20 creatures with high power
10 disposable creatures with high power
10 cards that like sacrificing high-power creatures
10 cards that like seeing high-power creatures enter and leave
10 cards that pump them
```

That's more than the 63 non-land slots, so the cards have to do several jobs each. Moxfield groups
the deck by tag, and that's all it can do with the tags. I can't ask it which of my ramp cards are
also creatures, or which cards hold two jobs.

### With Tutor

The Scryfall search and the deck sit side by side in one window. I scroll the search, and the card's
text, rulings and tags show beside it. `a` puts a card in the deck. When I want the next query, `i`
edits the search in place, and the deck stays where it is.

The cards already know their Scryfall Tagger tags, so I don't have to tag removal by hand. My own tags
go on many cards at once. And the statistics panel answers the questions Moxfield couldn't: open it,
pick *creature* AND *ramp*, and the deck narrows to those cards with the counts redrawn for them.

## How it's organized

Tutor is a row of panels, with an info panel on the right that describes whatever is highlighted.
`h` `l` (or the arrow keys) move between panels, `j` `k` move within one.

| Keys | Opens |
| --- | --- |
| `space d` | your lists: local decks, Moxfield decks, tag lists |
| `space f` | a Scryfall search |
| `space r` | the comprehensive rules |
| `space n` | a new panel; `tab` cycles between the three |

From the lists panel you open a local list in a panel of its own. In a Scryfall search panel you
query Scryfall and get a list of results.

One of your local lists is the **editing list**. It has its own border color, and `e` `E` choose
which one it is. `a` adds the highlighted card to it and `x` removes it, from whichever panel you're
in, so you can add from a search two panels over. Cards that are already in the editing list are
marked in the other panels.

`?` shows every key for wherever you are. `space` shows the menu. `q` quits.

![A search panel beside the editing deck, with cards already in the deck marked](screenshots/add-to-deck.png)

### Filtering

`/` narrows the list in front of you as you type. Plain words match the name, the text, the type
line and the tags. It also takes some of Scryfall's syntax, worked out on your machine over the
cards in the list:

```
t:creature  o:"draw a card"  mv<=3  pow>=4  c:rg  id:gruul  r>=rare  usd<5
f:commander  kw:flying  set:mh3  otag:ramp  tag:wincon
-t:land  (t:instant or t:sorcery)
```

`tag:` is your own tags, including the ones from tag lists. Anything it doesn't know is matched as
plain text.

### Sorting

Every list has two sorts. `.` cycles the first one, which fills the column on the right: mana value,
color, type, power, toughness, EDHREC rank, price, rarity. `,` cycles the second one, which breaks
ties and colors the names. Sorted by mana value and then by type, the curve reads from top to bottom
with the creatures at each cost together. `alt+.` and `alt+,` reverse them.

The *inclusion* sort puts the cards already in your editing list first, then the ones in another
list on screen, then the rest.

![A search sorted by mana value and then by type, names colored by type](screenshots/sorts.png)

## Git version control

Your whole library is a git repository. Each list is a plain text file, every saved version is kept,
any version can be brought back, and the library can be synced to a remote like GitHub.

`w` commits the active list. `space w` commits every list with changes. Edits are written to the file
as you make them, so nothing is lost if you forget.

To see a list's history:

1. `space d` to open the lists panel.
2. `j` `k` to scroll to the list.
3. `gv` to see its versions, with the diff of each one in the info panel.
4. `r` to go back to that version, or `c` to make a copy of it.

![A deck's version history, with the diff of the selected version in the info panel](screenshots/deck-git.png)

To sync, make an empty **private** repository on any git host (GitHub, Codeberg, GitLab, your own
server) and connect it once: in the settings (`space c`), `enter` on *git remote*, or from the shell:

```bash
ttr sync remote git@github.com:you/mtg-decks.git
```

After that, `space s` (or `ttr sync`) pushes your library and pulls in what you changed on another
machine. If the same list was changed in two places, git reports a conflict for you to resolve. It
never overwrites one side silently.

## Moxfield

You can browse anyone's public Moxfield decks and make local copies of them.

1. `space d` to open the lists panel.
2. `i`, and type a Moxfield username (or paste a deck URL).
3. `j` `k` to scroll through their public decks.
4. `enter` to open a deck, `c` to make a local copy of the main deck, or `C` to copy the main deck
   and its considering list.

The people and decks you follow stay under a `moxfield` folder at the top of the lists panel.

![The lists panel with local decks and followed Moxfield decks](screenshots/decks.png)

<!-- recording: following a Moxfield user and copying a deck -->

## Statistics

The statistics panel shows bar charts of a list: your tags, card types, colors, mana values (X
spells get a row of their own), rarity, price, and last the Scryfall Tagger tags. `enter` on a card
type opens it into the subtypes in the list: the creature types under Creature, Equipment under
Artifact. Build a filter from the rows, and the list and the bars narrow to the cards that match.

| Key | Does |
| --- | --- |
| `s` | open (or close) the statistics of the active list |
| `S` | open the statistics of the editing list |
| `j` `k` | walk through the rows |
| `J` `K` | jump between groups |
| `a` | add AND *row* to the filter |
| `o` | add OR *row* to the filter |
| `n` | add AND NOT *row* to the filter |
| `x` | take *row* out of the filter |
| `X` | clear the filter |
| `p` | show the odds of drawing each row in your opening hand |

You can still move between lists with `h` `l` and change the editing list with `e` `E` while the
panel is open.

For example, `s`, then `a` on *creature*, `o` on *artifact* and `n` on *ramp* filters the list to
`(creature OR artifact) AND NOT ramp`.

![The statistics panel, filtered to a combination of categories](screenshots/stats-filter.png)

`p` swaps the counts for the chance of at least one card from each row in your opening seven. Press
it again for at least two, three or four.

![Opening-hand odds for each tag, type and color](screenshots/odds.png)

## Tagging

- Tag many cards at once.
- Keep tag lists, and use any number of them in your deck.
- Copy tags from one list to another.

Tags are written at the end of a card's line in the list file: `1 Lightning Bolt (2xm) 141 [removal, burn]`.
Without the tags, a list pastes straight into Moxfield or Archidekt.

### Making a tag list

1. `space f` to open a new Scryfall search.
2. Search `id:rbg otag:removal`. Scryfall's API returns at most 175 cards a page, and Tutor
   loads the first page: by default, the 175 most played on EDHREC.
3. `w` to save the result as a local list, say `jund-removal`.
4. `V` to select every card in the list.
5. `t` to tag them, say `removal`.
6. `w` to commit.

### Using it in a deck

1. `space d` to open the lists panel.
2. Scroll to `jund-removal` and press `t` to make it a tag list. The tag lists in use show under a
   `tag lists` folder at the top.
3. Scroll to your Jund deck and press `enter` to open it.
4. `s` to open the statistics. The cards in the deck that are in `jund-removal` count as `removal`
   in the tags group, and the filters find them too.

The tags aren't written into the deck. To write them in, press `T` then `g`.

![Selecting cards in a search and tagging them together](screenshots/tagging.png)

### Copying tags between lists

`T` copies tags from a whole list at once: the cards you selected with `v`, or every card showing.
Filter first to copy only some. `T t` and `T a` ask which tags to bring: type one or a few (`tab`
completes from the tags in the list), or leave it empty for all of them.

| Keys | Does |
| --- | --- |
| `T t` | this list's tags onto the editing list, for the cards it has |
| `T a` | the same, and the cards the editing list doesn't have are added with their tags |
| `T g` | the tag lists' tags, written into this list |
| `T m` | every list on screen gets the others' tags, for the cards it has |

## Scryfall Tagger tags

The [Scryfall Tagger](https://tagger.scryfall.com/) community has tagged a lot of cards.
Birds of Paradise, for instance, is tagged
[activated-ability](https://tagger.scryfall.com/tags/card/activated-ability),
[cycle-lea-slush-art](https://tagger.scryfall.com/tags/card/cycle-lea-slush-art),
[evasion](https://tagger.scryfall.com/tags/card/evasion),
[mana-dork](https://tagger.scryfall.com/tags/card/mana-dork),
[real-life-animal-name](https://tagger.scryfall.com/tags/card/real-life-animal-name) and
[type-errata-specific-bird](https://tagger.scryfall.com/tags/card/type-errata-specific-bird).
On Scryfall you can search for them with
[`otag:mana-dork`](https://scryfall.com/search?q=otag%3Amana-dork). It's fantastic work, and I use it
all the time when building decks.

What I was missing was a way to use these tags to organize a deck. Scryfall doesn't show the tags of
a single card. But Scryfall makes its data available for [download](https://scryfall.com/docs/api/bulk-data),
tags included. Once a week Tutor downloads the tags (about 6 MB: 4,560 tags on 36,000 cards) and turns
them around, so each card's tags can be looked up. That takes about 3 MB on disk.

The tags show up in three places:

- The info panel lists the tags of the highlighted card.
- The statistics have a Scryfall Tagger group. Tags are nested (`removal` holds `removal-creature`,
  `removal-artifact`, `sweeper` and so on), and `enter` opens a tag to show the ones under it.
  What a tag means shows under the highlighted one, where Tagger describes it (about a third of the
  tags have a description).
- In a Scryfall search, `tab` after `otag:` completes the tag name.

There are two ways to turn Tagger tags into tags of your own on a local list.

From the `i` bar:

1. In your list, press `i`, then `tab` to switch to tagging by otag.
2. Type the tag, `removal`, and press `enter`.
3. Scryfall is asked which cards in your list have `otag:removal`, and those cards are tagged
   `otag-removal`.

From the statistics:

1. In your list, press `s` to open the statistics.
2. `K` to the Scryfall Tagger group (it's the last group, one step up from the top), then `j` to
   the tag you want.
3. `a` to filter the list by it, and `s` to close the statistics.
4. `V` to select every card left, and `t` to tag them.
5. Type a name for the tag and press `enter`.

![The info panel showing a card's printing and its Tagger tags](screenshots/printing.png)

<!-- screenshot: deck with the Tagger group open in the statistics -->
<!-- recording: tagging a deck by otag -->

## The card

The info panel shows the highlighted card's oracle text, with keywords highlighted, its rulings and
its legalities.

`gx` shows the card as printed, in the printing your list holds. `H` and `L` step through older and
newer artworks, `f` turns a double-faced card over, and `K` `J` scroll down to the price, tags and
rulings. The picture follows the cursor, and `gx` again puts the card back. `gX` downloads the pictures of every card in the list, so
walking through it is instant. Pictures are drawn in the terminal in kitty, Ghostty and WezTerm. In
other terminals `gx` opens the card in your browser.

![A Scryfall search with the info panel showing oracle text, rulings and legalities](screenshots/rulings-inline.png)

### Card history

`gv` on a card shows its printed text on every printing it has had, so you can see the errata and
the wording changes over the years. The text comes from [MTGJSON](https://mtgjson.com/), one file
per set, about 1.5 MB each. The sets you don't have yet download straight away, newest first, and the
history fills in as they arrive. Each set is kept, so a card reprinted in many sets is slow only the
first time.

![A card's text compared across its printings](screenshots/text-history.png)

## Comprehensive rules

Tutor downloads the comprehensive rules from Wizards of the Coast and parses them. Scryfall's lists
of keywords add the ones the rules don't name on their own, like Forestwalk and Plainscycling, which
are forms of landwalk and cycling. Together they highlight keywords in the oracle text, and each
keyword leads to its rule.

`space r` opens the rules panel. With a card highlighted, it lists the rules that card uses: its
keywords and the terms in its text. Scroll the list, and the info panel shows the full rule. `/`
searches all the rules.

`s` in the rules panel checks for a new version of the rules. When one is downloaded, Tutor keeps the
version it replaces, and `gv` shows what changed between them, rule by rule.

From the shell, `ttr rules <query>` searches the rules too.

![The rules a card uses, with the full rule in the info panel](screenshots/rules.png)

## Size and speed

Measured on my laptop (Intel i7-1355U, Linux):

- One 14 MB binary.
- It opens in about 20 ms, with the panels you left back on screen. A Scryfall search among them
  arrives a quarter of a second later.
- A key press is drawn in about 13 ms.
- It uses 45 to 60 MB of memory.
- What it downloads is kept on disk: about 40 MB, half of it every card's rulings so they show
  without asking Scryfall, plus about 100 KB for each card picture you've looked at. `ttr cache`
  shows what's there.

Anything that waits, waits on Scryfall, which asks apps to send no more than ten requests and two
searches a second. Tutor keeps to that.

## Install

Tutor is one binary, `ttr`, with nothing else to install. Git is needed for version control and
sync.

### Prebuilt binaries

Download the file for your machine from the [latest release](https://github.com/marbris/tutor/releases):

| Platform | File |
| --- | --- |
| Linux (x86-64) | `ttr-linux-amd64` |
| Linux (ARM64) | `ttr-linux-arm64` |
| macOS (Apple Silicon) | `ttr-darwin-arm64` |
| macOS (Intel) | `ttr-darwin-amd64` |
| Windows (x86-64) | `ttr-windows-amd64.exe` |

Linux:

```bash
chmod +x ttr-linux-amd64
mv ttr-linux-amd64 ~/.local/bin/ttr   # ~/.local/bin needs to be on your PATH
```

macOS:

```bash
chmod +x ttr-darwin-arm64
xattr -d com.apple.quarantine ttr-darwin-arm64   # removes the "unidentified developer" block
mv ttr-darwin-arm64 /usr/local/bin/ttr
```

Windows: rename `ttr-windows-amd64.exe` to `ttr.exe` and put it somewhere on your `PATH`.

### From source

Needs [Go 1.26+](https://go.dev/dl/).

```bash
git clone https://github.com/marbris/tutor.git
cd tutor
go build -o ttr .
ln -s "$(pwd)/ttr" ~/.local/bin/ttr
```

`make all` builds every platform into `dist/`.

## Command line

`ttr` on its own opens the panels you left, sorted and filtered the way they were. It also works
without the interface:

```bash
ttr 't:creature c:rw cmc<=3'          # open a Scryfall search
ttr Isshin                            # one exact match prints the card in the terminal
ttr https://moxfield.com/decks/...    # open a Moxfield deck
```

Searches use [Scryfall's syntax](https://scryfall.com/docs/syntax).

![ttr printing a card in the terminal](screenshots/quick-lookup.png)

The subcommands each have a `-h`:

```bash
ttr deck list                   # your lists
ttr deck new <name> [format]    # an empty deck
ttr deck import <id|url> [as]   # copy a Moxfield deck
ttr deck log <name>             # the git history of a list
ttr deck restore <name> <ref>   # bring back an earlier version
ttr deck dir                    # where your lists are on disk

ttr sync                        # push and pull your lists
ttr sync remote <url>           # connect a git remote
ttr sync status                 # what's ahead or behind
ttr sync off                    # disconnect (your lists are untouched)

ttr rules <query>               # search the comprehensive rules
ttr theme [name]                # list themes, or switch
ttr keys [--defaults]           # list the key bindings, or print them as a keys.json
ttr init                        # write commented-out templates of every settings file
ttr cache                       # what's downloaded and how much room it takes
ttr cache clear [kind]          # delete it, all of it or one kind
```

## Keys

`?` shows the keys for wherever you are, along the bottom of each panel.

<details>
<summary><b>Full key reference</b></summary>

**Getting around**

| Key | Does |
| --- | --- |
| `h` `l` / `←` `→` | previous / next panel |
| `ctrl+h` `ctrl+l` | move the panel along the row |
| `j` `k` | down / up |
| `gg` `G` | first / last row |
| `K` `J` | scroll the info panel |
| `b` `B` | clear this list's filters / every list's filters |
| `space` | the menu |
| `?` | show the keys |
| `q` | quit |

**In a list of cards**

| Key | Does |
| --- | --- |
| `/` | filter as you type: plain words, or some of Scryfall's syntax (see *Filtering*) |
| `.` `>` | next / previous first sort |
| `,` `<` | next / previous second sort |
| `alt+.` `alt+,` | reverse the first / second sort |
| `i` | edit the search; on your own list, add a card (`tab` completes its name; on an empty bar, `tab` tags by otag instead) |
| `v` `V` | select one / select all |
| `a` `A` | add to the editing list / add and tag with the last tag |
| `x` | remove a copy from the editing list |
| `t` | tag the selection (`tab` completes) |
| `T` `t` `a` `g` `m` | copy tags between lists |
| `c` | make the card the editing deck's commander |
| `u` | undo |
| `y` `p` | yank the selection / put it into this list |
| `s` `S` | statistics of this list / of the editing list |
| `gv` | the card's text history |
| `gx` `gX` | the card as printed / and download every picture in the list |
| `H` `L` | in the printing: older / newer artwork |
| `f` | in the printing: the other face |
| `w` `W` | commit, or save a search or Moxfield deck as your own (`W` opens it) |

**In the lists panel**

| Key | Does |
| --- | --- |
| `enter` | open a folder or a list |
| `L` | open the list in a new panel |
| `i` | follow a Moxfield user or deck |
| `n` `r` | new list / rename |
| `c` `C` | copy / copy with its considering list |
| `y` `x` `p` | yank / cut / put into this folder |
| `d` | delete |
| `t` | use as a tag list, or stop |
| `gv` | git history |
| `gx` | open a Moxfield deck on moxfield.com |

Lists are grouped into folders by their path: renaming a list to `aggro/mono-red` moves it into
`aggro`.

**The editing list and panels**

| Key | Does |
| --- | --- |
| `e` `E` | choose the editing list |
| `gd` | jump to the editing list |
| `space f` `space d` `space r` | new search / lists / rules panel |
| `space n` | new panel |
| `space x` `space o` | close this panel / close the others |
| `space c` | the settings |
| `space u` | bring back the last closed panel |
| `space w` | commit every list with changes |
| `space s` | sync with the git remote |

</details>

## Settings

`space c` opens the settings panel:

- **sync**: the git remote your lists mirror to. `enter` sets it, `d` disconnects.
- **downloads**: the files Tutor keeps from Scryfall: the Tagger tags, every card's rulings, and its
  lists of keywords, types and card names. Each has its size, and `enter` turns it off or on. Off,
  Tutor does without it: rulings are fetched a card at a time, for instance.
- **cache**: what's downloaded, by kind, and how much room each takes. `d` clears one. Everything
  there can be downloaded again.

The rest is in files. `ttr init` writes every settings file with the defaults commented out. They change nothing until you
uncomment a line.

- `~/.config/ttr/keys.json`: rebind any key. `ttr keys --defaults` prints the whole keymap. A clash
  is reported on start, and that scope keeps its defaults until it's fixed.
- `~/.config/ttr/config.json`: which sorts `.` and `,` step through, in what order, and which way
  each one starts.
- Themes: `ttr theme` lists them (gruvbox, nord, and one that uses your terminal's colors), and
  `ttr theme <name>` switches. You can add your own.

Settings files may have `//` comments.

## Where your files live

| | Holds | Linux | macOS | Windows |
| --- | --- | --- | --- | --- |
| **Data** | your lists (back this up) | `~/.local/share/ttr` | `~/Library/Application Support/ttr` | `%AppData%\ttr` |
| **Config** | settings, keys, themes | `~/.config/ttr` | `~/Library/Application Support/ttr` | `%AppData%\ttr` |
| **State** | session, search history | `~/.local/state/ttr` | `~/Library/Application Support/ttr` | `%AppData%\ttr` |
| **Cache** | downloaded card data, pictures, rules | `~/.cache/ttr` | `~/Library/Caches/ttr` | `%LocalAppData%\ttr` |

`TTR_DECKS_DIR` keeps your lists somewhere else. Tutor used to be called scry; files from then are
moved to these directories on the first run.

To uninstall, delete the binary and these directories. The data directory holds your lists, so copy
it first if you want to keep them.

## Credits

Tutor is built on other people's free data:

- [Scryfall](https://scryfall.com/): card data, pictures and search, through its [API](https://scryfall.com/docs/api).
- The [Scryfall Tagger](https://tagger.scryfall.com/) community: the oracle tags.
- [Moxfield](https://moxfield.com/): public decks.
- [MTGJSON](https://mtgjson.com/): the printed text of every printing.
- [EDHREC](https://edhrec.com/): Commander play rankings, the default search order.
- Wizards of the Coast: the [comprehensive rules](https://magic.wizards.com/en/rules).

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss) from [Charm](https://charm.sh/).

## License

MIT, see [LICENSE](LICENSE).

*Magic: The Gathering is © Wizards of the Coast. Tutor is an unofficial fan project, not produced by,
endorsed by or affiliated with Wizards of the Coast.*

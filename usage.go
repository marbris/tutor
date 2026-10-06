package main

import "fmt"

const cliUsage = `Tutor (ttr) — Magic: The Gathering cards, rules and decks in the terminal

Usage:
  ttr                      Come back to the panels you left
  ttr <query>               Run a Scryfall query; one result prints to stdout
  ttr <moxfield url>        Look at a deck on Moxfield
  ttr deck …                Your decks — see ` + "`ttr deck`" + `
  ttr sync …                Mirror your decks to a git remote — see ` + "`ttr sync`" + `
  ttr rules …               The comprehensive rules — see ` + "`ttr rules`" + `
  ttr theme …               Colours — see ` + "`ttr theme`" + `
  ttr keys …                Keybindings — see ` + "`ttr keys -h`" + `
  ttr launcher [remove]     Put Tutor in the application launcher, or take it out
  ttr init                  Write commented-out templates of every settings file
  ttr cache                 What's downloaded, and how much room it takes; clear empties it
  ttr -v, --version         Print the version
  ttr -h, --help            This

Queries use Scryfall's own syntax:
  ttr 't:creature c:rw cmc<=3'
  ttr 'o:"draw a card" f:commander'

The app is a row of panels. space opens the menu, ? shows the keys.

Files follow the XDG directories: decks in the data directory, settings,
keys and themes in the config directory, session state in the state
directory, and everything re-downloadable in the cache. Decks are files in a
git repository — TTR_DECKS_DIR moves them somewhere else.`

func printUsage() { fmt.Println(cliUsage) }

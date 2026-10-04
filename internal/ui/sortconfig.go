package ui

import (
	"fmt"
	"strings"

	"ttr/internal/config"
)

// The orders as config.json sets them. Like keys.json, a setting that can't
// be used is reported and the shipped behaviour stands in for it — a typo in
// one order's name shouldn't cost you the rest.

// sortNamed is the order a name in config.json means. The orders answer to
// what the header calls them, and the order the cards came in to the two
// names it goes by.
func sortNamed(name string) (cardSort, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "scryfall", "scryfall order", "arrival", "as found", "decklist":
		return sortArrival, true
	case "colour":
		return sortColor, true
	case "mana", "cmc", "mv":
		return sortMana, true
	case "price":
		return sortUSD, true
	}
	for _, s := range allSorts {
		if s != sortArrival && s.String() == name {
			return s, true
		}
	}
	return 0, false
}

// SetSortConfig puts config.json's sort settings in force. A nil setting
// puts back the shipped ones.
func SetSortConfig(c *config.Sort) error {
	sortCycle, sortDescending = defaultCycle, defaultDescending
	if c == nil {
		return nil
	}

	var problems []string
	if len(c.Cycle) > 0 {
		var cycle []cardSort
		seen := map[cardSort]bool{}
		for _, name := range c.Cycle {
			s, ok := sortNamed(name)
			if !ok {
				problems = append(problems, fmt.Sprintf("no sort %q", name))
				continue
			}
			if !seen[s] {
				seen[s] = true
				cycle = append(cycle, s)
			}
		}
		if len(cycle) > 0 {
			sortCycle = cycle
		}
	}

	if len(c.Direction) > 0 {
		desc := map[cardSort]bool{}
		for s, d := range defaultDescending {
			desc[s] = d
		}
		for name, dir := range c.Direction {
			s, ok := sortNamed(name)
			if !ok {
				problems = append(problems, fmt.Sprintf("no sort %q", name))
				continue
			}
			switch strings.ToLower(dir) {
			case "asc", "ascending", "up":
				desc[s] = false
			case "desc", "descending", "down":
				desc[s] = true
			default:
				problems = append(problems, fmt.Sprintf("%s: direction %q is neither asc nor desc", name, dir))
			}
		}
		sortDescending = desc
	}

	if len(problems) > 0 {
		return fmt.Errorf("%s: sort: %s", config.Path(), strings.Join(problems, ", "))
	}
	return nil
}

// SortDefaults is the shipped sort settings, as config.json would hold them.
func SortDefaults() config.Sort {
	c := config.Sort{Direction: map[string]string{}}
	for _, s := range defaultCycle {
		c.Cycle = append(c.Cycle, sortConfigName(s))
	}
	for _, s := range allSorts {
		dir := "asc"
		if defaultDescending[s] {
			dir = "desc"
		}
		c.Direction[sortConfigName(s)] = dir
	}
	return c
}

// sortConfigName is what config.json calls an order.
func sortConfigName(s cardSort) string {
	if s == sortArrival {
		return "scryfall"
	}
	return s.String()
}

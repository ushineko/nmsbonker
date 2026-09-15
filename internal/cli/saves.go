package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/save"
)

// newSavesCmd is the saves group: backup (spec 004 R5.2) and the editor
// (spec 007 R7).
func newSavesCmd() *cobra.Command {
	cmd := group("saves", "Back up, inspect and edit the game's saves")
	cmd.AddCommand(newSavesBackupCmd(), newSavesListCmd(),
		newSavesSlotsCmd(), newSavesInspectCmd(), newSavesExportCmd(), newSavesImportCmd(), newSavesEditCmd(),
		newSavesGetCmd(), newSavesSetCmd())
	return cmd
}

func newSavesBackupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backup",
		Short: "Copy every save profile to a timestamped folder",
		Long: "Copies the st_* folders out of the game's Proton prefix into nmsbonker's own\n" +
			"backup directory. Restoring is a manual copy; `saves edit` and `saves import`\n" +
			"take one of these automatically before they write.",
		Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.BackupSaves(cmd.Context(), core.BackupSavesRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if res.Skipped != "" {
				fact(w, "skipped", res.Skipped)
				return nil
			}
			fact(w, "source", res.Source)
			fact(w, "backup", res.Dir)
			fact(w, "profiles", res.Profiles)
			fact(w, "files", fmt.Sprintf("%d (%s)", res.Files, humanSize(res.Bytes)))
			if len(res.Pruned) > 0 {
				fact(w, "pruned", fmt.Sprintf("%d older backup(s): %s", len(res.Pruned), join(res.Pruned)))
			}
			return nil
		},
	}
}

func newSavesListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the save backups, newest first",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListSaveBackups(cmd.Context(),
				core.ListSaveBackupsRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "backup dir", res.Dir)
			fact(w, "retention", fmt.Sprintf("the newest %d are kept", res.Retention))
			fact(w, "before a deploy", yesNo(res.Enabled)+" (config save_backup)")
			if len(res.Backups) == 0 {
				say(w, "%s", "No save backups yet.")
				return nil
			}
			var t table
			t.header("TIMESTAMP", "PROFILES", "FILES", "SIZE")
			for _, b := range res.Backups {
				t.row(b.Timestamp, fmt.Sprintf("%d", b.Profiles),
					fmt.Sprintf("%d", b.Files), humanSize(b.Bytes))
			}
			t.write(w)
			if res.Source != "" {
				say(w, "")
				say(w, "%s", "To restore one, close the game and copy an st_* folder back to:")
				say(w, "  %s", res.Source)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

// --- the editor (spec 007 R7) -----------------------------------------------

// slotArg turns the positional slot argument into a selector, as a usage error
// when it is not one.
func slotArg(s string) (core.SlotSelector, error) {
	sel, err := core.ParseSlotSelector(s)
	if err != nil {
		return core.SlotSelector{}, usagef("%v", err)
	}
	return sel, nil
}

func newSavesSlotsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "slots",
		Short: "List the save slots the game has, and which it would load",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListSaveSlots(cmd.Context(), core.ListSaveSlotsRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "profile", res.Profile)
			if len(res.Profiles) > 1 {
				fact(w, "other profiles", join(res.Profiles[1:]))
			}
			fact(w, "mapping", mappingText(res.Mapping))
			fact(w, "game running", yesNo(res.GameRunning))
			if len(res.Slots) == 0 {
				say(w, "%s", "No saves in this profile.")
				return nil
			}
			var t table
			t.header("", "SLOT", "KIND", "NAME", "SUMMARY", "PLAYED", "VERSION", "WRITTEN")
			for _, sl := range res.Slots {
				marker := ""
				if sl.Newest {
					marker = "*"
				}
				t.row(marker, fmt.Sprintf("%d", sl.Slot), string(sl.Kind), orDash(sl.Name),
					orDash(firstOf(sl.Summary, sl.MetaError)), playTime(sl.PlayTime),
					versionText(sl.BaseVersion, sl.GameMode), writtenText(sl))
			}
			t.write(w)
			say(w, "")
			say(w, "%s", "* is the save the game would load. `saves inspect 9` reads a slot; add :auto or :manual to pick a half.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newSavesInspectCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "inspect <slot>[:auto|:manual]",
		Short: "Show what the editor can see in one save, without writing anything",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := slotArg(args[0])
			if err != nil {
				return err
			}
			res, err := core.InspectSave(cmd.Context(), core.InspectSaveRequest{Request: request(), Slot: sel})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			s := res.Summary
			fact(w, "save", fmt.Sprintf("slot %d %s (%s)", res.Ref.Slot, res.Ref.Kind, res.File))
			fact(w, "name", orDash(firstOf(res.Meta.Name, s.SaveName)))
			fact(w, "summary", orDash(res.Meta.Summary))
			fact(w, "played", playTime(uint64(max(s.PlayTime, 0)))) //nolint:gosec // clamped
			fact(w, "version", fmt.Sprintf("%d = base %d, %s", s.Version, s.Base, save.GameModeName(s.GameMode)))
			fact(w, "platform", s.Platform)
			fact(w, "state", s.StatePath)
			fact(w, "units", fmt.Sprintf("%d", s.Units))
			fact(w, "nanites", fmt.Sprintf("%d", s.Nanites))
			fact(w, "quicksilver", fmt.Sprintf("%d", s.Quicksilver))
			fact(w, "health / shield", fmt.Sprintf("%d / %d", s.Health, s.Shield))
			fact(w, "suit item slots", inventoryText(s.SuitItems))
			fact(w, "suit tech slots", inventoryText(s.SuitTech))
			fact(w, "ships / multitools", fmt.Sprintf("%d / %d", s.Ships, s.Multitools))
			for _, sh := range s.ShipList {
				fact(w, "  ship "+strconv.Itoa(sh.Index), fmt.Sprintf("%s · type %s · items %s · tech %s",
					sh.Label(), orDash(sh.Type), inventoryText(sh.Items), inventoryText(sh.Tech)))
			}
			if s.Freighter.Present {
				fact(w, "freighter", fmt.Sprintf("type %s · class %s · items %s · tech %s",
					orDash(s.Freighter.Type), orDash(s.Freighter.Class), inventoryText(s.Freighter.Items), inventoryText(s.Freighter.Tech)))
			}
			fact(w, "standing", standingText(s.Standings))
			fact(w, "mapping", mappingText(res.Mapping))
			if len(s.Unmapped) > 0 {
				fact(w, "unmapped keys", fmt.Sprintf("%d (%s)", len(s.Unmapped), join(firstN(s.Unmapped, 8))))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newSavesExportCmd() *cobra.Command {
	var (
		out           string
		pretty, names bool
	)
	cmd := &cobra.Command{
		Use:   "export <slot>[:auto|:manual]",
		Short: "Write a save's JSON to a file outside the game folder",
		Long: "Decodes the save and writes its JSON. --names replaces the game's obfuscated\n" +
			"keys with readable ones; `saves import` turns them back. The file goes under\n" +
			"the workspace unless --out says otherwise, and never into the save folder.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := slotArg(args[0])
			if err != nil {
				return err
			}
			res, err := core.ExportSave(cmd.Context(), core.ExportSaveRequest{
				Request: request(), Slot: sel, Out: out, Pretty: pretty, Names: names,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "save", fmt.Sprintf("slot %d %s (%s)", res.Ref.Slot, res.Ref.Kind, res.File))
			fact(w, "wrote", fmt.Sprintf("%s (%s)", res.Out, humanSize(res.Bytes)))
			if names {
				fact(w, "keys left obfuscated", res.Unnamed)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "file to write (default: under the workspace)")
	cmd.Flags().BoolVar(&pretty, "pretty", false, "indent the JSON")
	cmd.Flags().BoolVar(&names, "names", false, "replace obfuscated keys with their names")
	return cmd
}

func newSavesImportCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "import <slot>[:auto|:manual] <file>",
		Short: "Replace a save's contents with a JSON file, backing the profile up first",
		Long: "Reads a JSON file -- typically one from `saves export`, edited -- and writes\n" +
			"it back as the slot's save. The whole profile is copied to the backup\n" +
			"directory first, the game must not be running, and the manifest is rewritten\n" +
			"to match.",
		Args: exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := slotArg(args[0])
			if err != nil {
				return err
			}
			res, err := core.ImportSave(cmd.Context(), core.ImportSaveRequest{
				Request: request(), Slot: sel, In: args[1], Force: force,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "save", fmt.Sprintf("slot %d %s", res.Ref.Slot, res.Ref.Kind))
			fact(w, "from", res.In)
			if res.Obfuscated > 0 {
				fact(w, "keys turned back", res.Obfuscated)
			}
			writeFacts(w, res.Write)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "write even if the game is running (unsafe: it may overwrite the edit)")
	return cmd
}

func newSavesEditCmd() *cobra.Command {
	var (
		units, nanites, quicksilver uint64
		health, shield              int64
		itemSlots, techSlots        int
		dryRun, force, asJSON       bool
		standings                   []string
		ship                        int
		shipItems, shipTech         int
		shipClass, freighterClass   string
		shipType, freighterType     string
		frItems, frTech             int
	)
	cmd := &cobra.Command{
		Use:   "edit <slot>[:auto|:manual] [--units N] [--nanites N] [--quicksilver N] [--suit-slots N] [--suit-tech-slots N] [--health N] [--shield N] [--standing FACTION=N]... [--ship I] [--ship-slots N] [--ship-tech-slots N] [--ship-class C|B|A|S] [--freighter-slots N] [--freighter-tech-slots N] [--freighter-class C|B|A|S]",
		Short: "Change currencies, vitals, slot counts, classes and faction standing in a save",
		Long: "Applies the changes named by the flags and writes the save back. The whole\n" +
			"profile is copied to the backup directory first, the game must not be running,\n" +
			"and the manifest is rewritten to match. --dry-run lists what would change.\n\n" +
			"Slot counts unlock slots up to the game's own ceiling for the inventory (120 items,\n" +
			"60 technology for the exosuit, every ship and the freighter, from the game's inventory\n" +
			"table); the grid grows a row at a time the way it does when slots are bought.\n\n" +
			"--standing sets a faction's standing to a level 1..9, as the reputation screen\n" +
			"shows it, repeatable: gek, korvax, vykeen, mercenaries, explorers or merchants,\n" +
			"e.g. --standing gek=7 --standing vykeen=3. The value written is the lowest that\n" +
			"shows as that level, which also sits just under the next level's gate.\n\n" +
			"--ship-slots, --ship-tech-slots and --ship-class act on the ship --ship names (an\n" +
			"index from `saves inspect`; default the ship you are flying). Classes set the badge\n" +
			"and the class gate on upgrades; the stat bonuses rolled with the ship are unchanged.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := slotArg(args[0])
			if err != nil {
				return err
			}
			var cs save.ChangeSet
			f := cmd.Flags()
			if f.Changed("units") {
				cs.Units = &units
			}
			if f.Changed("nanites") {
				cs.Nanites = &nanites
			}
			if f.Changed("quicksilver") {
				cs.Quicksilver = &quicksilver
			}
			if f.Changed("health") {
				cs.Health = &health
			}
			if f.Changed("shield") {
				cs.Shield = &shield
			}
			if f.Changed("suit-slots") {
				cs.SuitItemSlots = &itemSlots
			}
			if f.Changed("suit-tech-slots") {
				cs.SuitTechSlots = &techSlots
			}
			for _, kv := range standings {
				key, val, ok := strings.Cut(kv, "=")
				n, convErr := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
				if !ok || convErr != nil {
					return usagef("--standing wants FACTION=N, got %q", kv)
				}
				if cs.Standings == nil {
					cs.Standings = map[string]int64{}
				}
				cs.Standings[strings.ToLower(strings.TrimSpace(key))] = n
			}
			cs.Ship = ship
			if f.Changed("ship-slots") {
				cs.ShipItemSlots = &shipItems
			}
			if f.Changed("ship-tech-slots") {
				cs.ShipTechSlots = &shipTech
			}
			if f.Changed("ship-class") {
				shipClass = strings.ToUpper(shipClass)
				cs.ShipClass = &shipClass
			}
			if f.Changed("freighter-slots") {
				cs.FreighterItemSlots = &frItems
			}
			if f.Changed("freighter-tech-slots") {
				cs.FreighterTechSlots = &frTech
			}
			if f.Changed("freighter-class") {
				freighterClass = strings.ToUpper(freighterClass)
				cs.FreighterClass = &freighterClass
			}
			if f.Changed("ship-type") {
				shipType = strings.ToLower(shipType)
				cs.ShipType = &shipType
			}
			if f.Changed("freighter-type") {
				freighterType = strings.ToLower(freighterType)
				cs.FreighterType = &freighterType
			}
			if cs.Empty() {
				return usagef("nothing to change: give at least one of --units, --nanites, --quicksilver, " +
					"--health, --shield, --suit-slots, --suit-tech-slots, --standing, --ship-slots, " +
					"--ship-tech-slots, --ship-class, --ship-type, --freighter-slots, --freighter-tech-slots, " +
					"--freighter-class, --freighter-type")
			}
			res, err := core.EditSave(cmd.Context(), core.EditSaveRequest{
				Request: request(), Slot: sel, Changes: cs, DryRun: dryRun, Force: force,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "save", fmt.Sprintf("slot %d %s (%s)", res.Ref.Slot, res.Ref.Kind, res.File))
			if len(res.Changes) == 0 {
				say(w, "%s", "Every value asked for is already what the save holds; nothing was written.")
				return nil
			}
			var t table
			t.header("FIELD", "OLD", "NEW", "PATH")
			for _, c := range res.Changes {
				t.row(c.Field, c.Old, c.New, c.Path)
			}
			t.write(w)
			if res.DryRun {
				say(w, "%s", "Dry run: nothing was written.")
				return nil
			}
			if res.Write != nil {
				writeFacts(w, *res.Write)
			}
			return nil
		},
	}
	cmd.Flags().Uint64Var(&units, "units", 0, "units (0..4294967295)")
	cmd.Flags().Uint64Var(&nanites, "nanites", 0, "nanites (0..4294967295)")
	cmd.Flags().Uint64Var(&quicksilver, "quicksilver", 0, "quicksilver (0..4294967295)")
	cmd.Flags().Int64Var(&health, "health", 0, "health; the game clamps values above its maximum")
	cmd.Flags().Int64Var(&shield, "shield", 0, "shield; the game clamps values above its maximum")
	cmd.Flags().IntVar(&itemSlots, "suit-slots", 0, "unlocked exosuit item slots (1..120)")
	cmd.Flags().IntVar(&techSlots, "suit-tech-slots", 0, "unlocked exosuit technology slots (1..60)")
	cmd.Flags().IntVar(&ship, "ship", -1, "which ship the --ship-* flags act on: its index from `saves inspect` (default: the one you fly)")
	cmd.Flags().IntVar(&shipItems, "ship-slots", 0, "unlocked starship item slots (1..120)")
	cmd.Flags().IntVar(&shipTech, "ship-tech-slots", 0, "unlocked starship technology slots (1..60)")
	cmd.Flags().StringVar(&shipClass, "ship-class", "", "starship class: C, B, A or S")
	cmd.Flags().IntVar(&frItems, "freighter-slots", 0, "unlocked freighter item slots (1..120)")
	cmd.Flags().IntVar(&frTech, "freighter-tech-slots", 0, "unlocked freighter technology slots (1..60)")
	cmd.Flags().StringVar(&freighterClass, "freighter-class", "", "freighter class: C, B, A or S")
	cmd.Flags().StringVar(&shipType, "ship-type", "", "starship type: "+typeKeys(save.ShipTypes)+" (the seed is kept; the look is regenerated)")
	cmd.Flags().StringVar(&freighterType, "freighter-type", "", "freighter type: "+typeKeys(save.FreighterTypes))
	cmd.Flags().StringArrayVar(&standings, "standing", nil, "faction standing level as FACTION=1..9; repeatable (gek, korvax, vykeen, mercenaries, explorers, merchants)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list the changes and write nothing")
	cmd.Flags().BoolVar(&force, "force", false, "write even if the game is running (unsafe: it may overwrite the edit)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newSavesGetCmd() *cobra.Command {
	var raw, asJSON bool
	cmd := &cobra.Command{
		Use:   "get <slot>[:auto|:manual] [PATH]",
		Short: "Print one node of a save's JSON, with its children listed",
		Long: "PATH is slash-separated names or keys, with a number for an array element, e.g.\n" +
			"BaseContext/PlayerStateData/DifficultyState. No path is the whole save. A node\n" +
			"larger than 256 KiB is not printed; its children are, so you can descend.",
		Args: minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 2 {
				return usagef("%s takes a slot and at most one path", cmd.CommandPath())
			}
			sel, err := slotArg(args[0])
			if err != nil {
				return err
			}
			path := ""
			if len(args) == 2 {
				path = args[1]
			}
			res, err := core.GetSaveNode(cmd.Context(), core.SaveNodeRequest{Request: request(), Slot: sel, Path: path, Raw: raw})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(w, res)
			}
			if res.JSON != "" {
				_, err := io.WriteString(w, res.JSON)
				return err //nolint:wrapcheck // the writer's own error
			}
			fact(w, "path", orDash(res.Path))
			fact(w, "type", fmt.Sprintf("%s, %s", res.Type, humanSize(int64(res.Bytes))))
			say(w, "%s", "Too large to print; its children:")
			var t table
			t.header("NAME", "TYPE", "SIZE", "ITEMS")
			for _, c := range res.Children {
				t.row(c.Name, c.Type, humanSize(int64(c.Bytes)), fmt.Sprintf("%d", c.Len))
			}
			t.write(w)
			return nil
		},
	}
	cmd.Flags().BoolVar(&raw, "raw", false, "keep the game's obfuscated keys")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newSavesSetCmd() *cobra.Command {
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:   "set <slot>[:auto|:manual] PATH <file|->",
		Short: "Replace one node of a save's JSON from a file (or stdin with -)",
		Long: "Parses the file as JSON, turns named keys back into the game's spelling, puts the\n" +
			"value at PATH in place of what is there, and writes the save through the same\n" +
			"backup-first path as `saves edit`. The path must already exist: this replaces a\n" +
			"value, it does not add one.",
		Args: exactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := slotArg(args[0])
			if err != nil {
				return err
			}
			var body []byte
			if args[2] == "-" {
				body, err = io.ReadAll(cmd.InOrStdin())
			} else {
				body, err = os.ReadFile(args[2])
			}
			if err != nil {
				return fmt.Errorf("read the new value: %w", err)
			}
			res, err := core.SetSaveNode(cmd.Context(), core.SetSaveNodeRequest{
				Request: request(), Slot: sel, Path: args[1], JSON: string(body), DryRun: dryRun, Force: force,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "save", fmt.Sprintf("slot %d %s", res.Ref.Slot, res.Ref.Kind))
			fact(w, "path", res.Path)
			if !res.Changed {
				say(w, "%s", "The node already holds that value; nothing was written.")
				return nil
			}
			if res.Obfuscated > 0 {
				fact(w, "keys turned back", res.Obfuscated)
			}
			if res.DryRun {
				say(w, "%s", "Dry run: the node would change; nothing was written.")
				return nil
			}
			if res.Write != nil {
				writeFacts(w, *res.Write)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "check the value and write nothing")
	cmd.Flags().BoolVar(&force, "force", false, "write even if the game is running (unsafe: it may overwrite the edit)")
	return cmd
}

// writeFacts prints what a guarded write did (R6.4).
func writeFacts(w io.Writer, res core.SaveWriteResult) {
	fact(w, "backup", res.Backup)
	if len(res.Pruned) > 0 {
		fact(w, "pruned", fmt.Sprintf("%d older backup(s): %s", len(res.Pruned), join(res.Pruned)))
	}
	fact(w, "wrote", fmt.Sprintf("%s (%s, %s decompressed)", res.DataFile, humanSize(res.Bytes), humanSize(int64(res.Decompressed))))
	fact(w, "manifest", res.MetaFile)
	if res.Forced {
		fact(w, "forced", "the game was running; its next autosave may overwrite this edit")
	}
	say(w, "")
	say(w, "%s", res.Note)
}

// standingText renders the six standings on one line.
func standingText(standings []save.StandingValue) string {
	if len(standings) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(standings))
	for _, sv := range standings {
		v := fmt.Sprintf("level %d (%d)", sv.Level, sv.Value)
		if !sv.Present {
			v = "—"
		}
		parts = append(parts, sv.Label+" "+v)
	}
	return strings.Join(parts, ", ")
}

// typeKeys lists a type table's keys for a flag's help.
func typeKeys(table []save.ShipType) string {
	keys := make([]string, 0, len(table))
	for _, t := range table {
		keys = append(keys, t.Key)
	}
	return strings.Join(keys, ", ")
}

func mappingText(st core.MappingStatus) string {
	if !st.Present {
		return "absent: " + st.Warning
	}
	return fmt.Sprintf("%s (libMBIN %s, %d keys)", st.Path, st.LibMBINVersion, st.Entries)
}

func inventoryText(inv save.InventorySummary) string {
	return fmt.Sprintf("%d unlocked (%d×%d grid), %d occupied", inv.Valid, inv.Width, inv.Height, inv.Occupied)
}

func versionText(base uint32, mode string) string {
	if base == 0 {
		return "—"
	}
	return fmt.Sprintf("%d %s", base, mode)
}

func writtenText(sl core.SaveSlotInfo) string {
	when := sl.Modified
	if sl.Timestamp > 0 {
		when = time.Unix(sl.Timestamp, 0)
	}
	return when.Local().Format("2006-01-02 15:04")
}

// playTime renders seconds as hours and minutes.
func playTime(seconds uint64) string {
	if seconds == 0 {
		return "—"
	}
	h, m := seconds/3600, (seconds%3600)/60
	return fmt.Sprintf("%dh%02dm", h, m)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstN(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return append(append([]string(nil), items[:n]...), "…")
}

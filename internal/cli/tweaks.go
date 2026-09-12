package cli

import (
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

// newTweaksCmd is the built-in tweaks group (spec 004 R2.3).
//
// Enabling and disabling go through the same core operation `mods enable` uses;
// they are spelled here as well because a user looking at `tweaks list` should
// not have to learn that a tweak is also a mod before they can turn one on.
func newTweaksCmd() *cobra.Command {
	cmd := group("tweaks", "Built-in mods with tunable parameters")
	cmd.AddCommand(
		newTweaksListCmd(),
		newTweaksSetCmd(),
		newTweaksResetCmd(),
		newTweaksEnableCmd(true),
		newTweaksEnableCmd(false),
	)
	return cmd
}

func newTweaksListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the built-in tweaks and their parameters",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListTweaks(cmd.Context(), core.ListTweaksRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			printTweaks(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func printTweaks(w io.Writer, res core.ListTweaksResult) {
	var t table
	t.header("#", "TWEAK", "GROUP", "ON", "PARAMETER", "VALUE", "DEFAULT", "RANGE")
	for _, tw := range res.Tweaks {
		for i, p := range tw.Params {
			name, group, on := "", "", ""
			if i == 0 {
				name, group, on = tw.Name, tw.Group, yesNo(tw.Enabled)
			}
			order := ""
			if i == 0 {
				order = strconv.Itoa(tw.Order)
			}
			t.row(order, name, group, on, p.Name,
				modscript.FormatValue(p.Current, p.Kind),
				modscript.FormatValue(p.Default, p.Kind), rangeText(p))
		}
	}
	t.write(w)
	say(w, "")
	for _, tw := range res.Tweaks {
		if tw.Shadowed {
			fact(w, "notice", tw.Name+": a library script of the same name is ignored; "+
				"`nmsbonker mods remove "+tw.Name+"` deletes it")
		}
	}
	if res.Unbuilt {
		fact(w, "unbuilt", "a parameter has changed since the last build; run `nmsbonker build`")
	}
	say(w, "%s", "Order is the build order, shared with every other mod: lower is applied later.")
}

// rangeText is the bounds column, or a note that there are none.
func rangeText(p core.TweakParam) string {
	if !p.Bounded {
		return "unbounded"
	}
	return fmt.Sprintf("%s..%s step %s",
		modscript.FormatValue(p.Min, p.Kind),
		modscript.FormatValue(p.Max, p.Kind),
		modscript.FormatValue(p.Step, p.Kind))
}

func newTweaksSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set NAME PARAM VALUE",
		Short: "Set one parameter of a built-in tweak or a library script",
		Args:  exactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := strconv.ParseFloat(args[2], 64)
			if err != nil {
				return usagef("VALUE must be a number, got %q", args[2])
			}
			res, err := core.SetTweakParam(cmd.Context(), core.SetTweakParamRequest{
				Request: request(), Name: args[0], Param: args[1], Value: value,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			say(w, "%s %s: %s -> %s (default %s)", res.Name, res.Param,
				trimFloat(res.Old), trimFloat(res.New), trimFloat(res.Default))
			if res.Clamped {
				fact(w, "note", fmt.Sprintf(
					"%s is outside the declared range, so %s was used instead",
					trimFloat(value), trimFloat(res.New)))
			}
			fact(w, "note", "the script on disk is unchanged; the value is applied at build time")
			return nil
		},
	}
}

func newTweaksResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset NAME [PARAM]",
		Short: "Restore a tweak's own values, for one parameter or all of them",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			param := ""
			if len(args) == 2 {
				param = args[1]
			}
			res, err := core.ResetTweak(cmd.Context(), core.ResetTweakRequest{
				Request: request(), Name: args[0], Param: param,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(res.Reset) == 0 {
				say(w, "%s was already at its own values", res.Name)
				return nil
			}
			say(w, "reset %s: %s", res.Name, join(res.Reset))
			return nil
		},
	}
}

func newTweaksEnableCmd(enable bool) *cobra.Command {
	verb, past, short := "enable", "enabled", "Include built-in tweaks in the next build"
	if !enable {
		verb, past, short = "disable", "disabled", "Leave built-in tweaks out of the next build"
	}
	return &cobra.Command{
		Use:   verb + " NAME...",
		Short: short,
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.SetModEnabled(cmd.Context(), core.SetModEnabledRequest{
				Request: request(), Names: args, Enabled: enable,
			})
			if err != nil {
				return err
			}
			for _, n := range res.Changed {
				say(cmd.OutOrStdout(), "%s %s", past, n)
			}
			return nil
		},
	}
}

// trimFloat prints a parameter value without a trailing ".0" on whole numbers,
// which is what makes `10 -> 20` read as a change of ten rather than of 10.0.
func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

-- @tweak name="Fleet takes no damage" group="Ships"
-- @desc Frigates never come back from an expedition damaged. A failed event
-- @desc still costs its rewards and still shows in the debrief, but no
-- @desc frigate needs repairing. Frigates already damaged stay damaged.
-- Two things damage a frigate. An ordinary failed event rolls against
-- PercentChanceOfDamageOnFailedEvent in GCFLEETGLOBALS, a range (stock 0 to
-- 20 percent) the chance climbs through as the frigate goes on more
-- expeditions. A failed intervention event (the choices offered while an
-- expedition is out) rolls against its own FailureDamageChance in the
-- expedition event table (stock 0, 10, 25, 50 or 100). Both go to zero.

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "FleetNoDamage.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "GCFLEETGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["PRECEDING_KEY_WORDS"] = {"PercentChanceOfDamageOnFailedEvent"},
                            ["VALUE_CHANGE_TABLE"]  = { {"X", 0}, {"Y", 0} }
                        }
                    }
                },
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\EXPEDITIONEVENTTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["PRECEDING_KEY_WORDS"] = "",
                            ["REPLACE_TYPE"]        = "ALL",
                            ["VALUE_CHANGE_TABLE"]  = { {"FailureDamageChance", 0} }
                        }
                    }
                }
            }
        }
    }
}

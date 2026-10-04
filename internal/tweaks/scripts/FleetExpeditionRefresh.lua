-- @tweak name="Fleet expedition refresh" group="Ships"
-- @desc New frigate expeditions are offered every so many minutes instead of
-- @desc once a real-world day, and the board can offer more of them at once.
-- @param MINUTES_PER_DAY label="Minutes between new expeditions" min=1 max=1440 step=1 default=10
-- @param CHOICES label="Expeditions offered at once" min=1 max=10 step=1 default=5
-- The expedition board is seeded by the day: the save keeps a real-world day
-- count (LastKnownDay) and the seeds already picked that day. GCFLEETGLOBALS
-- carries OverrideExpeditionSecondsPerDay, -1 (off) in the shipped file; this
-- sets it, so a "day" lasts MINUTES_PER_DAY. NumberOfExpeditionChoices is how
-- many expeditions the board offers (stock 5).
MINUTES_PER_DAY = 10  -- stock: one real-world day
CHOICES = 5           -- stock 5

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "FleetExpeditionRefresh.pak",
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
                            ["VALUE_CHANGE_TABLE"] =
                            {
                                {"OverrideExpeditionSecondsPerDay", MINUTES_PER_DAY * 60},
                                {"NumberOfExpeditionChoices",       CHOICES}
                            }
                        }
                    }
                }
            }
        }
    }
}

-- @tweak name="Fleet expedition time" group="Ships"
-- @desc Frigate expeditions finish sooner: each event on an expedition takes
-- @desc this many seconds, on normal and easy difficulty alike. After Ahawk's
-- @desc "FleetUpdate".
-- @param EVENT_SECONDS label="Seconds per expedition event" min=1 max=5400 step=1 default=5
EVENT_SECONDS = 5  -- stock 5400 (normal) and 900 (easy)

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "FleetExpeditionTime.pak",
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
                                {"TimeTakenForExpeditionEvent",      EVENT_SECONDS},
                                {"TimeTakenForExpeditionEvent_Easy", EVENT_SECONDS}
                            }
                        }
                    }
                }
            }
        }
    }
}

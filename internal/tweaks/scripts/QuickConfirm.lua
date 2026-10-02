-- @tweak name="Quick confirm" group="Interface"
-- @desc Hold-to-confirm buttons fill in this many seconds, and holding with
-- @desc the mouse takes no longer than with a key. After Lo2k's
-- @desc "SpeedIncreaseActions".
-- @param CONFIRM_SECONDS label="Hold time, seconds" min=0.05 max=1 step=0.05 default=0.15 kind=float
CONFIRM_SECONDS = 0.15  -- stock 0.7 (0.35 for the fast ones)

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "QuickConfirm.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "GCUIGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["VALUE_CHANGE_TABLE"] =
                            {
                                {"FrontendConfirmTime",                CONFIRM_SECONDS},
                                {"FrontendConfirmTimeFast",            CONFIRM_SECONDS},
                                {"FrontendConfirmTimeMouseMultiplier", 1}
                            }
                        }
                    }
                }
            }
        }
    }
}

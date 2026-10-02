-- @tweak name="Movement speed" group="Player"
-- @desc Running, the jetpack and swimming are faster on foot. After the
-- @desc "runforrestrun" mod.
-- @param RUN_MULT label="Running speed multiplier" min=1 max=10 step=0.5 default=3 kind=float
-- @param JETPACK_MULT label="Jetpack speed multiplier" min=1 max=20 step=0.5 default=10 kind=float
-- @param SWIM_MULT label="Swimming speed multiplier" min=1 max=10 step=0.5 default=3 kind=float
RUN_MULT     = 3   -- GroundRunSpeed x this (stock 8)
JETPACK_MULT = 10  -- JetpackMaxSpeed x this (stock 5)
SWIM_MULT    = 3   -- UnderwaterSwimMaxSpeed x this (stock 4)

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "MovementSpeed.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "GCPLAYERGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["MATH_OPERATION"]     = "*",
                            ["VALUE_CHANGE_TABLE"] =
                            {
                                {"GroundRunSpeed",         RUN_MULT},
                                {"JetpackMaxSpeed",        JETPACK_MULT},
                                {"UnderwaterSwimMaxSpeed", SWIM_MULT}
                            }
                        }
                    }
                }
            }
        }
    }
}

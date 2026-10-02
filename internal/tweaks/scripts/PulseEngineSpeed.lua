-- @tweak name="Pulse engine speed" group="Ships"
-- @desc Pulse drive cruises faster between planets. After BigEx20's
-- @desc "AtmoHover-PulseSpeedDefined".
-- @param PULSE_MULT label="Pulse speed multiplier" min=1 max=20 step=0.5 default=4 kind=float
PULSE_MULT = 4  -- MiniWarpSpeed x this

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "PulseEngineSpeed.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "GCSPACESHIPGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["MATH_OPERATION"]     = "*",
                            ["VALUE_CHANGE_TABLE"] = { {"MiniWarpSpeed", PULSE_MULT} }
                        }
                    }
                }
            }
        }
    }
}

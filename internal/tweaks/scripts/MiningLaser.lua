-- @tweak name="Mining laser" group="Gathering"
-- @desc The multitool's beam mines faster, and digging into the terrain gives
-- @desc more of the ground it removes. After "runforrestrun" and Mjjstral's
-- @desc "ScanTimesAndRangeImprove".
-- @param LASER_RATE label="Beam mining rate multiplier" min=1 max=20 step=1 default=7
-- @param TERRAIN_YIELD label="Terrain yield multiplier" min=1 max=50 step=1 default=10
LASER_RATE    = 7   -- LaserBeamMineRate x this (stock 0.3)
TERRAIN_YIELD = 10  -- terrain-resource min/max amounts x this

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "MiningLaser.pak",
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
                            ["VALUE_CHANGE_TABLE"] = { {"LaserBeamMineRate", LASER_RATE} }
                        }
                    }
                },
                {
                    ["MBIN_FILE_SOURCE"] = "GCGAMEPLAYGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["MATH_OPERATION"]     = "*",
                            ["VALUE_CHANGE_TABLE"] =
                            {
                                {"ResourceMinAmount",       TERRAIN_YIELD},
                                {"ResourceMaxAmount",       TERRAIN_YIELD},
                                {"ResourceCommonMinAmount", TERRAIN_YIELD},
                                {"ResourceCommonMaxAmount", TERRAIN_YIELD},
                                {"ResourceDirtMinAmount",   TERRAIN_YIELD},
                                {"ResourceDirtMaxAmount",   TERRAIN_YIELD}
                            }
                        }
                    }
                }
            }
        }
    }
}

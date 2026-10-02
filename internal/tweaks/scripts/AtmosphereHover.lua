-- @tweak name="Atmosphere hover" group="Ships"
-- @desc A ship in atmosphere can slow to a standstill and hover instead of
-- @desc keeping a minimum speed, in every ship class. After BigEx20's
-- @desc "AtmoHover-PulseSpeedDefined".

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "AtmosphereHover.pak",
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
                            ["VALUE_CHANGE_TABLE"] =
                            {
                                {"HoverTakeoffHeight", 80},
                                {"HoverSpeedFactor",   0.0001},
                                {"HoverMinSpeed",      0.0001}
                            }
                        },
                        {
                            -- One PlanetEngine section per ship class, and only
                            -- those: the space and boost engines keep their own.
                            ["SPECIAL_KEY_WORDS"]  = {"PlanetEngine"},
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"MinSpeed", 0.0001} }
                        }
                    }
                }
            }
        }
    }
}

-- @tweak name="Scanner" group="Player"
-- @desc The analysis visor identifies in a moment, and the multitool's and the
-- @desc ship's scanners reach further and recharge sooner. After Mjjstral's
-- @desc "ScanTimesAndRangeImprove".
-- @param VISOR_SECONDS label="Analysis visor scan time, seconds" min=0 max=4 step=0.1 default=0 kind=float
-- @param SCAN_RANGE label="Scan range multiplier" min=1 max=10 step=0.5 default=2 kind=float
-- @param SCAN_RECHARGE label="Scanner recharge, seconds" min=1 max=30 step=1 default=2
VISOR_SECONDS = 0  -- every binocular scan time is set to this
SCAN_RANGE    = 2  -- tool, hard-mode tool and ship PulseRange x this
SCAN_RECHARGE = 2  -- tool, hard-mode tool and ship ChargeTime set to this

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ScannerBoost.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "GCGAMEPLAYGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["VALUE_CHANGE_TABLE"] =
                            {
                                {"BinocMinScanTime",      VISOR_SECONDS},
                                {"BinocScanTime",         VISOR_SECONDS},
                                {"BinocCreatureScanTime", VISOR_SECONDS}
                            }
                        }
                    }
                },
                {
                    -- The scanners moved out of GCGAMEPLAYGLOBALS into this table. Each
                    -- entry is anchored on its quoted _id: keywords are matched in
                    -- sequence, so {"ID", "TOOL"} would run on past this entry.
                    ["MBIN_FILE_SOURCE"] = "METADATA\SIMULATION\SCANNING\SCANDATATABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"TOOL"},
                            ["MATH_OPERATION"]     = "*",
                            ["VALUE_CHANGE_TABLE"] = { {"PulseRange", SCAN_RANGE} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"TOOL"},
                            ["VALUE_CHANGE_TABLE"] = { {"ChargeTime", SCAN_RECHARGE} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"TOOL_HARD"},
                            ["MATH_OPERATION"]     = "*",
                            ["VALUE_CHANGE_TABLE"] = { {"PulseRange", SCAN_RANGE} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"TOOL_HARD"},
                            ["VALUE_CHANGE_TABLE"] = { {"ChargeTime", SCAN_RECHARGE} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"SHIP"},
                            ["MATH_OPERATION"]     = "*",
                            ["VALUE_CHANGE_TABLE"] = { {"PulseRange", SCAN_RANGE} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"SHIP"},
                            ["VALUE_CHANGE_TABLE"] = { {"ChargeTime", SCAN_RECHARGE} }
                        }
                    }
                }
            }
        }
    }
}

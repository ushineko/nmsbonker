-- @tweak name="Big stacks" group="Inventory"
-- @desc Raises the inventory stack limits, for every difficulty tier and every
-- @desc inventory. Absolute caps rather than a multiplier: products start at
-- @desc five to twenty and the game uses hard limits.
-- @param SUBSTANCE label="Substance stack limit" min=1 max=9999999 step=1000 default=999999
-- @param PRODUCT label="Product stack limit" min=1 max=9999999 step=1000 default=99999
-- Big inventory stacks: raise the difficulty stack caps AND the per-inventory
-- max stack sizes across ALL difficulty tiers, so heavy looting/mining doesn't
-- flood the inventory. Absolute caps (not a small multiplier) since products
-- start tiny (5-20) and NMS uses hard limits.
SUBSTANCE = 999999
PRODUCT   = 99999

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "BigStacks.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\GAMESTATE\DIFFICULTYCONFIG.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        -- difficulty tier caps (3 tiers) -> raise both
                        {
                            ["PRECEDING_KEY_WORDS"] = "",
                            ["REPLACE_TYPE"]        = "ALL",
                            ["VALUE_CHANGE_TABLE"]  = { {"SubstanceStackLimit", SUBSTANCE}, {"ProductStackLimit", PRODUCT} }
                        },
                        -- per-inventory substance max, every MaxSubstanceStackSizes block
                        {
                            ["SPECIAL_KEY_WORDS"] = {"MaxSubstanceStackSizes"},
                            ["REPLACE_TYPE"]      = "ALL",
                            ["VALUE_CHANGE_TABLE"]= {
                                {"Default",SUBSTANCE},{"Personal",SUBSTANCE},{"Ship",SUBSTANCE},{"Freighter",SUBSTANCE},
                                {"Vehicle",SUBSTANCE},{"Chest",SUBSTANCE},{"BaseCapsule",SUBSTANCE},
                                {"MaintenanceObject",SUBSTANCE},{"UIPopup",SUBSTANCE}
                            }
                        },
                        -- per-inventory product max, every MaxProductStackSizes block
                        {
                            ["SPECIAL_KEY_WORDS"] = {"MaxProductStackSizes"},
                            ["REPLACE_TYPE"]      = "ALL",
                            ["VALUE_CHANGE_TABLE"]= {
                                {"Default",PRODUCT},{"Personal",PRODUCT},{"Ship",PRODUCT},{"Freighter",PRODUCT},
                                {"Vehicle",PRODUCT},{"Chest",PRODUCT},{"BaseCapsule",PRODUCT},{"MaintenanceObject",PRODUCT}
                            }
                        }
                    }
                }
            }
        }
    }
}

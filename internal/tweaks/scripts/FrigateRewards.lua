-- @tweak name="Frigate rewards" group="Ships"
-- @desc Multiplies what frigate expeditions bring back: units, nanites,
-- @desc products and raw materials, each on its own multiplier. After
-- @desc MrTrack's "BetterFrigateRewards".
-- @param FRIGATE_UNITS label="Units multiplier" min=1 max=100 step=1 default=10 scales="units"
-- @param FRIGATE_NANITES label="Nanites multiplier" min=1 max=100 step=1 default=10 scales="nanites"
-- @param FRIGATE_PRODUCTS label="Products multiplier" min=1 max=100 step=1 default=10 scales="product"
-- @param FRIGATE_SUBSTANCES label="Raw materials multiplier" min=1 max=100 step=1 default=10 scales="substance"
FRIGATE_UNITS      = 10
FRIGATE_NANITES    = 10
FRIGATE_PRODUCTS   = 10
FRIGATE_SUBSTANCES = 10

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "FrigateRewards.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\EXPEDITIONREWARDTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"Currency", "Units"},
                            ["SECTION_UP"]         = 1,
                            ["MATH_OPERATION"]     = "*",
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"AmountMin", FRIGATE_UNITS}, {"AmountMax", FRIGATE_UNITS} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"Currency", "Nanites"},
                            ["SECTION_UP"]         = 1,
                            ["MATH_OPERATION"]     = "*",
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"AmountMin", FRIGATE_NANITES}, {"AmountMax", FRIGATE_NANITES} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"DefaultProductType", "None"},
                            ["SECTION_UP"]         = 2,
                            ["MATH_OPERATION"]     = "*",
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"AmountMin", FRIGATE_PRODUCTS}, {"AmountMax", FRIGATE_PRODUCTS} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"DefaultSubstanceType", "None"},
                            ["SECTION_UP"]         = 2,
                            ["MATH_OPERATION"]     = "*",
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"AmountMin", FRIGATE_SUBSTANCES}, {"AmountMax", FRIGATE_SUBSTANCES} }
                        }
                    }
                }
            }
        }
    }
}

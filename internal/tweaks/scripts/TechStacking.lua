-- @tweak name="Technology stacking" group="Player"
-- @desc How many upgrade modules of one kind can be installed together and all
-- @desc count. The game allows three. After Wbertro's
-- @desc "StackingTechnologyModules".
-- @param TECH_STACK label="Modules of one kind that count" min=3 max=50 step=1 default=25
TECH_STACK = 25  -- MaxNumSameGroupTech (stock 3)

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "TechStacking.pak",
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
                        { ["VALUE_CHANGE_TABLE"] = { {"MaxNumSameGroupTech", TECH_STACK} } }
                    }
                }
            }
        }
    }
}

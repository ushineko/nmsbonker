-- @tweak name="Refiner speed" group="Gathering"
-- @desc Every refiner recipe finishes sooner: its time to make is divided by
-- @desc the speed. What goes in and what comes out are unchanged. After
-- @desc wim95's "FastRefiners".
-- @param REFINER_SPEED label="Refining speed" min=1 max=100 step=1 default=10
REFINER_SPEED = 10  -- divide every recipe's TimeToMake by this

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "RefinerSpeed.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\NMS_REALITY_GCRECIPETABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["MATH_OPERATION"]     = "/",
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"TimeToMake", REFINER_SPEED} }
                        }
                    }
                }
            }
        }
    }
}

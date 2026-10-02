-- @tweak name="Ship transfer range" group="Ships"
-- @desc How far from your ship you can still move items to and from its
-- @desc inventory. The game allows 50 metres. After Lo2k's "Better Ship
-- @desc Transfer Range".
-- @param TRANSFER_RANGE label="Transfer range, metres" min=50 max=1000000 step=50 default=1000000
TRANSFER_RANGE = 1000000  -- ShipInteractRadius (stock 50)

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ShipTransferRange.pak",
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
                        { ["VALUE_CHANGE_TABLE"] = { {"ShipInteractRadius", TRANSFER_RANGE} } }
                    }
                }
            }
        }
    }
}

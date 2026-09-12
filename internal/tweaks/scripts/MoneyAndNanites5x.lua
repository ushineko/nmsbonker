-- @tweak name="Units and nanites" group="Currency"
-- @desc Multiplies every Units and Nanites reward exactly once per reward
-- @desc block, so a mixed units-and-nanites jackpot is not multiplied twice.
-- @param CUR_MULT label="Units and nanites multiplier" min=1 max=100 step=1 default=5
-- Multiply Units AND Nanites reward amounts x5. Uses the deterministic
-- CURRENCY_MULT op (each GcRewardMoney block multiplied exactly once by currency),
-- which avoids the double-application that SKW matching caused in complex mixed
-- units+nanites reward entries (e.g. mission-board MEGA jackpots).
CUR_MULT = 5

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "MoneyAndNanites5x.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\REWARDTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        { ["CURRENCY_MULT"] = { ["CURRENCY"]="Units",   ["MULT"]=CUR_MULT } },
                        { ["CURRENCY_MULT"] = { ["CURRENCY"]="Nanites", ["MULT"]=CUR_MULT } }
                    }
                }
            }
        }
    }
}

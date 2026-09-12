-- @tweak name="Units and nanites" group="Currency"
-- @desc Multiplies every Units and Nanites reward exactly once per reward
-- @desc block, so a mixed units-and-nanites jackpot is not multiplied twice.
-- @param CUR_MULT label="Units and nanites multiplier" min=1 max=100 step=1 default=5
-- @param UNITS_CAP label="Largest units reward" min=0 max=2000000000 step=1000000 default=50000000
-- @param NANITES_CAP label="Largest nanites reward" min=0 max=10000000 step=10000 default=250000
-- Multiply Units AND Nanites reward amounts x5. Uses the deterministic
-- CURRENCY_MULT op (each GcRewardMoney block multiplied exactly once by currency),
-- which avoids the double-application that SKW matching caused in complex mixed
-- units+nanites reward entries (e.g. mission-board MEGA jackpots).
CUR_MULT    = 5
UNITS_CAP   = 50000000  -- ceiling on the final units amount. 0 = no ceiling.
NANITES_CAP = 250000    -- ceiling on the final nanites amount. 0 = no ceiling.

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
                        { ["CURRENCY_MULT"] = { ["CURRENCY"]="Units",   ["MULT"]=CUR_MULT }, ["CAP"] = UNITS_CAP },
                        { ["CURRENCY_MULT"] = { ["CURRENCY"]="Nanites", ["MULT"]=CUR_MULT }, ["CAP"] = NANITES_CAP }
                    }
                }
            }
        }
    }
}

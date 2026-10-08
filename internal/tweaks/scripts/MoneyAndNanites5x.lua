-- @tweak name="Units and nanites" group="Rewards"
-- @desc Multiplies every Units and Nanites reward exactly once per reward
-- @desc block, so a mixed units-and-nanites jackpot is not multiplied twice.
-- @param CUR_MULT label="Units and nanites multiplier" min=1 max=100 step=1 default=5 scales="units,nanites"
-- @param UNITS_CAP label="Largest units reward" min=0 max=2000000000 step=1000000 default=50000000
-- @param NANITES_CAP label="Largest nanites reward" min=0 max=10000000 step=10000 default=250000
-- Multiply Units AND Nanites reward amounts x5. Written with the engine's own
-- SPECIAL_KEY_WORDS matching so the script is plain AMUMSS and runs anywhere:
-- the Currency/Units and Currency/Nanites anchors with SECTION_UP 2 each scope
-- to one GcRewardMoney block, so a mixed units-and-nanites jackpot is still
-- multiplied once per currency. CAP is the one key only nmsbonker reads.
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
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"Currency", "Units"},
                            ["SECTION_UP"]         = 2,
                            ["MATH_OPERATION"]     = "*",
                            ["REPLACE_TYPE"]       = "ALL",
                            ["CAP"]                = UNITS_CAP,
                            ["VALUE_CHANGE_TABLE"] = { {"AmountMin", CUR_MULT}, {"AmountMax", CUR_MULT} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"Currency", "Nanites"},
                            ["SECTION_UP"]         = 2,
                            ["MATH_OPERATION"]     = "*",
                            ["REPLACE_TYPE"]       = "ALL",
                            ["CAP"]                = NANITES_CAP,
                            ["VALUE_CHANGE_TABLE"] = { {"AmountMin", CUR_MULT}, {"AmountMax", CUR_MULT} }
                        }
                    }
                }
            }
        }
    }
}

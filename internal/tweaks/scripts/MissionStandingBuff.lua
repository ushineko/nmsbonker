-- @tweak name="Mission standing" group="Standing"
-- @desc Multiplies the faction and race standing a mission pays out.
-- @param STANDING_MULT label="Standing multiplier" min=1 max=50 step=1 default=5
-- @param STANDING_CAP label="Largest standing reward" min=0 max=100000 step=50 default=500
-- Buff STANDING/reputation from missions x5. Deterministic WRAPPER_MULT:
-- multiplies AmountMin/AmountMax once inside each GcRewardFactionStanding
-- (guild/faction) and GcRewardStanding (race) block in REWARDTABLE.
STANDING_MULT = 5
STANDING_CAP  = 500  -- ceiling on the resulting standing amount. 0 = no ceiling.

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "MissionStandingBuff.pak",
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
                        { ["WRAPPER_MULT"] = { ["WRAPPER"]="GcRewardFactionStanding", ["MULT"]=STANDING_MULT }, ["CAP"] = STANDING_CAP },
                        { ["WRAPPER_MULT"] = { ["WRAPPER"]="GcRewardStanding", ["MULT"]=STANDING_MULT }, ["CAP"] = STANDING_CAP }
                    }
                }
            }
        }
    }
}

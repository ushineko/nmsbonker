-- @tweak name="Chest and loot materials" group="Loot"
-- @desc Multiplies the substance and product amounts handed out by the reward
-- @desc tables: chests, cargo drops, salvage, containers and mission material
-- @desc rewards. Units and nanites are deliberately left alone.
-- @param LOOT_MULTIPLIER label="Reward amount multiplier" min=1 max=100 step=1 default=10
-- @param LOOT_CAP label="Largest reward amount" min=0 max=1000000 step=1000 default=50000
LOOT_MULTIPLIER = 10  -- multiply MATERIAL amounts (substance + product) from reward tables
                      -- (chests, cargo drops, salvage, containers, mission material rewards).
                      -- Units/nanites (GcRewardMoney) are intentionally left unchanged.
LOOT_CAP = 50000      -- ceiling on the result, whatever ran before this. 0 = no ceiling.
                      -- It is a ceiling on the FINAL amount, so a library script that has
                      -- already multiplied the same reward is clamped here too.

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ChestAndLootMaterials10x.pak",
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
                            ["SPECIAL_KEY_WORDS"] = {"GcRewardSpecificSubstance"},
                            ["MATH_OPERATION"]    = "*",
                            ["REPLACE_TYPE"]      = "ALL",
                            ["CAP"]               = LOOT_CAP,
                            ["VALUE_CHANGE_TABLE"]= { {"AmountMin", LOOT_MULTIPLIER}, {"AmountMax", LOOT_MULTIPLIER} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"] = {"GcRewardSpecificProduct"},
                            ["MATH_OPERATION"]    = "*",
                            ["REPLACE_TYPE"]      = "ALL",
                            ["CAP"]               = LOOT_CAP,
                            ["VALUE_CHANGE_TABLE"]= { {"AmountMin", LOOT_MULTIPLIER}, {"AmountMax", LOOT_MULTIPLIER} }
                        }
                    }
                }
            }
        }
    }
}

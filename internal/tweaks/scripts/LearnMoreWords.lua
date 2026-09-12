-- @tweak name="Learn more words" group="Language"
-- @desc Multiplies the number of words learned from a monolith, a knowledge
-- @desc stone, a plaque or a conversation.
-- @param WORD_MULT label="Words per interaction" min=1 max=50 step=1 default=5
-- Learn more words per interaction (monoliths, knowledge stones, plaques, NPCs).
-- Word grants are GcRewardTeachWord entries in REWARDTABLE with AmountMin/AmountMax
-- (COSMOS 7.x has a real count field; almost all standard word entries are 1).
-- Multiply x5 so normal sources give 5 (story-mission word rewards, which are
-- already >1, scale up too rather than being flattened).
WORD_MULT = 5

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "LearnMoreWords.pak",
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
                            ["SPECIAL_KEY_WORDS"] = {"GcRewardTeachWord"},
                            ["MATH_OPERATION"]    = "*",
                            ["REPLACE_TYPE"]      = "ALL",
                            ["VALUE_CHANGE_TABLE"]= { {"AmountMin", WORD_MULT}, {"AmountMax", WORD_MULT} }
                        }
                    }
                }
            }
        }
    }
}

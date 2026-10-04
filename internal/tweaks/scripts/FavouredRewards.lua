-- @tweak name="Favoured rewards" group="Rewards"
-- @desc Makes chosen items come up more often wherever a reward is picked from
-- @desc a list: mission board, Nexus and frigate expedition rewards among
-- @desc them. Each slider multiplies that item's weight; 1 is stock.
-- @param STORAGE_WEIGHT label="Storage Augmentation weight" min=1 max=100 step=1 default=5
-- @param MULTITOOL_WEIGHT label="Multi-tool Expansion Slot weight" min=1 max=100 step=1 default=5
-- @param EXOSUIT_WEIGHT label="Exosuit Expansion Unit weight" min=1 max=100 step=1 default=5
-- @param SPAWNING_SAC_WEIGHT label="Spawning Sac weight" min=1 max=100 step=1 default=5
-- @param FRIGATE_MODULE_WEIGHT label="Salvaged Frigate Module weight" min=1 max=100 step=1 default=5
-- @param REACTOR_WEIGHT label="S- and A-Class Reactor weight" min=1 max=100 step=1 default=5
-- @param ULTRAPROD_WEIGHT label="Fusion Ignitor and Stasis Device weight" min=1 max=100 step=1 default=5
-- @param STORM_CRYSTAL_WEIGHT label="Storm Crystal weight" min=1 max=100 step=1 default=5
-- @param AMOUNT_MULT label="Amount multiplier for these items" min=1 max=100 step=1 default=1 scales="product"
-- Most of these rewards are SelectAlways lists: one item is picked, and each
-- item's PercentageChance is a weight against the rest of the list, not a
-- percentage (R_MB_MEGA's add up to 159). Multiplying one item's weight makes
-- it that much likelier against the others; in the few GiveAll lists the
-- number is the item's own chance, and above 100 it is certain. The item is
-- found by its ID line; three sections up is the GcRewardTableItem holding
-- both its PercentageChance and, inside, its amounts.
STORAGE_WEIGHT        = 5   -- SHIP_INV_TOKEN, ~4% of a mission board reward
MULTITOOL_WEIGHT      = 5   -- WEAP_INV_TOKEN
EXOSUIT_WEIGHT        = 5   -- SUIT_INV_TOKEN
SPAWNING_SAC_WEIGHT   = 5   -- ALIEN_INV_TOKEN, frigate "whale" expeditions only
FRIGATE_MODULE_WEIGHT = 5   -- FRIG_TOKEN
REACTOR_WEIGHT        = 5   -- SHIP_CORE_S, SHIP_CORE_A
ULTRAPROD_WEIGHT      = 5   -- ULTRAPROD1, ULTRAPROD2, Nexus only
STORM_CRYSTAL_WEIGHT  = 5   -- STORM_CRYSTAL
AMOUNT_MULT           = 1

local function favour(ids, weight)
    local out = {}
    for _, id in ipairs(ids) do
        -- One keyword holding the whole line. The ALL path searches each
        -- keyword from the line after the last, so {"ID", id} as a pair
        -- skips every entry whose own ID line is the first "ID" it meets.
        -- This also leaves out the Id of a bundle (GcMultiSpecificItemEntry).
        table.insert(out, {
            ["SPECIAL_KEY_WORDS"]  = {'name="ID" value="' .. id .. '"'},
            ["SECTION_UP"]         = 3,
            ["MATH_OPERATION"]     = "*",
            ["REPLACE_TYPE"]       = "ALL",
            ["VALUE_CHANGE_TABLE"] = {
                {"PercentageChance", weight},
                {"AmountMin", AMOUNT_MULT},
                {"AmountMax", AMOUNT_MULT}
            }
        })
    end
    return out
end

local function concat(...)
    local out = {}
    for _, t in ipairs({...}) do
        for _, v in ipairs(t) do table.insert(out, v) end
    end
    return out
end

-- Each table lists only the items it holds, so a key the build cannot find
-- means the game moved it rather than that it was never there.
local reward = concat(
    favour({"SHIP_INV_TOKEN"}, STORAGE_WEIGHT),
    favour({"WEAP_INV_TOKEN"}, MULTITOOL_WEIGHT),
    favour({"SUIT_INV_TOKEN"}, EXOSUIT_WEIGHT),
    favour({"FRIG_TOKEN"}, FRIGATE_MODULE_WEIGHT),
    favour({"SHIP_CORE_S", "SHIP_CORE_A"}, REACTOR_WEIGHT),
    favour({"ULTRAPROD1", "ULTRAPROD2"}, ULTRAPROD_WEIGHT),
    favour({"STORM_CRYSTAL"}, STORM_CRYSTAL_WEIGHT))
local expedition = concat(
    favour({"SHIP_INV_TOKEN"}, STORAGE_WEIGHT),
    favour({"FRIG_TOKEN"}, FRIGATE_MODULE_WEIGHT),
    favour({"ALIEN_INV_TOKEN"}, SPAWNING_SAC_WEIGHT))

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "FavouredRewards.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\REWARDTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] = reward
                },
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\EXPEDITIONREWARDTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] = expedition
                }
            }
        }
    }
}

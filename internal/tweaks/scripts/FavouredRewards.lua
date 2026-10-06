-- @tweak name="Favoured rewards" group="Rewards"
-- @desc Makes chosen items come up more often wherever a reward is picked from
-- @desc a list: mission board, Nexus missions and every frigate expedition.
-- @desc Each slider multiplies that item's weight; 1 is stock.
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
--
-- Weighting only helps where the item is listed (spec 020). Nexus missions
-- all pay from R_NEXUS_MED, which of these holds only the frigate module, and
-- each expedition event pays from its own success list, most of which hold
-- none of them. So the items are first added to those lists, at a base weight
-- that gives each the share it has elsewhere (a Storage Augmentation about
-- 7%) against that list's stock total, and the weighting below then
-- multiplies them like any other.
STORAGE_WEIGHT        = 5   -- SHIP_INV_TOKEN, ~4% of a mission board reward
MULTITOOL_WEIGHT      = 5   -- WEAP_INV_TOKEN
EXOSUIT_WEIGHT        = 5   -- SUIT_INV_TOKEN
SPAWNING_SAC_WEIGHT   = 5   -- ALIEN_INV_TOKEN
FRIGATE_MODULE_WEIGHT = 5   -- FRIG_TOKEN
REACTOR_WEIGHT        = 5   -- SHIP_CORE_S, SHIP_CORE_A
ULTRAPROD_WEIGHT      = 5   -- ULTRAPROD1, ULTRAPROD2
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

-- For each list, the items it lacks and the base weight each goes in at:
-- the item's usual share times the list's stock weight total. Worked out from
-- the stock tables of game build 25625620.
local NEXUS_ADDS = {
    {"R_NEXUS_MED", {  -- stock weights add up to 134.5
        {"SHIP_INV_TOKEN", 9.15},
        {"WEAP_INV_TOKEN", 5.92},
        {"SUIT_INV_TOKEN", 4.04},
        {"ALIEN_INV_TOKEN", 1.34},
        {"SHIP_CORE_S", 0.81},
        {"SHIP_CORE_A", 1.75},
        {"ULTRAPROD1", 0.4},
        {"ULTRAPROD2", 0.4},
        {"STORM_CRYSTAL", 1.08},
    }},
}
local EXPEDITION_ADDS = {
    {"R_DIPLOMATIC_0", {  -- stock weights add up to 1056
        {"WEAP_INV_TOKEN", 46.46},
        {"SUIT_INV_TOKEN", 31.68},
        {"ALIEN_INV_TOKEN", 10.56},
        {"FRIG_TOKEN", 71.81},
        {"SHIP_CORE_S", 6.34},
        {"SHIP_CORE_A", 13.73},
        {"ULTRAPROD1", 3.17},
        {"ULTRAPROD2", 3.17},
        {"STORM_CRYSTAL", 8.45},
    }},
    {"R_DIPLOMATIC_1", {  -- stock weights add up to 100
        {"SHIP_INV_TOKEN", 6.8},
        {"WEAP_INV_TOKEN", 4.4},
        {"SUIT_INV_TOKEN", 3},
        {"ALIEN_INV_TOKEN", 1},
        {"FRIG_TOKEN", 6.8},
        {"SHIP_CORE_S", 0.6},
        {"SHIP_CORE_A", 1.3},
        {"ULTRAPROD1", 0.3},
        {"ULTRAPROD2", 0.3},
        {"STORM_CRYSTAL", 0.8},
    }},
    {"R_DIPLOMATIC_2", {  -- stock weights add up to 100
        {"SHIP_INV_TOKEN", 6.8},
        {"WEAP_INV_TOKEN", 4.4},
        {"SUIT_INV_TOKEN", 3},
        {"ALIEN_INV_TOKEN", 1},
        {"FRIG_TOKEN", 6.8},
        {"SHIP_CORE_S", 0.6},
        {"SHIP_CORE_A", 1.3},
        {"ULTRAPROD1", 0.3},
        {"ULTRAPROD2", 0.3},
        {"STORM_CRYSTAL", 0.8},
    }},
    {"R_DIPLOMATIC_3", {  -- stock weights add up to 100
        {"SHIP_INV_TOKEN", 6.8},
        {"WEAP_INV_TOKEN", 4.4},
        {"SUIT_INV_TOKEN", 3},
        {"ALIEN_INV_TOKEN", 1},
        {"FRIG_TOKEN", 6.8},
        {"SHIP_CORE_S", 0.6},
        {"SHIP_CORE_A", 1.3},
        {"ULTRAPROD1", 0.3},
        {"ULTRAPROD2", 0.3},
        {"STORM_CRYSTAL", 0.8},
    }},
    {"R_COMBAT_0", {  -- stock weights add up to 100
        {"SHIP_INV_TOKEN", 6.8},
        {"WEAP_INV_TOKEN", 4.4},
        {"SUIT_INV_TOKEN", 3},
        {"ALIEN_INV_TOKEN", 1},
        {"FRIG_TOKEN", 6.8},
        {"SHIP_CORE_S", 0.6},
        {"SHIP_CORE_A", 1.3},
        {"ULTRAPROD1", 0.3},
        {"ULTRAPROD2", 0.3},
        {"STORM_CRYSTAL", 0.8},
    }},
    {"R_COMBAT_1", {  -- stock weights add up to 605
        {"SHIP_INV_TOKEN", 41.14},
        {"WEAP_INV_TOKEN", 26.62},
        {"SUIT_INV_TOKEN", 18.15},
        {"ALIEN_INV_TOKEN", 6.05},
        {"FRIG_TOKEN", 41.14},
        {"SHIP_CORE_S", 3.63},
        {"SHIP_CORE_A", 7.86},
        {"ULTRAPROD1", 1.81},
        {"ULTRAPROD2", 1.81},
        {"STORM_CRYSTAL", 4.84},
    }},
    {"R_COMBAT_2", {  -- stock weights add up to 100
        {"SHIP_INV_TOKEN", 6.8},
        {"WEAP_INV_TOKEN", 4.4},
        {"SUIT_INV_TOKEN", 3},
        {"ALIEN_INV_TOKEN", 1},
        {"FRIG_TOKEN", 6.8},
        {"SHIP_CORE_S", 0.6},
        {"SHIP_CORE_A", 1.3},
        {"ULTRAPROD1", 0.3},
        {"ULTRAPROD2", 0.3},
        {"STORM_CRYSTAL", 0.8},
    }},
    {"R_EXPLORATION_0", {  -- stock weights add up to 220
        {"SHIP_INV_TOKEN", 14.96},
        {"WEAP_INV_TOKEN", 9.68},
        {"SUIT_INV_TOKEN", 6.6},
        {"ALIEN_INV_TOKEN", 2.2},
        {"FRIG_TOKEN", 14.96},
        {"SHIP_CORE_S", 1.32},
        {"SHIP_CORE_A", 2.86},
        {"ULTRAPROD1", 0.66},
        {"ULTRAPROD2", 0.66},
        {"STORM_CRYSTAL", 1.76},
    }},
    {"R_EXPLORATION_1", {  -- stock weights add up to 410
        {"SHIP_INV_TOKEN", 27.88},
        {"WEAP_INV_TOKEN", 18.04},
        {"SUIT_INV_TOKEN", 12.3},
        {"ALIEN_INV_TOKEN", 4.1},
        {"FRIG_TOKEN", 27.88},
        {"SHIP_CORE_S", 2.46},
        {"SHIP_CORE_A", 5.33},
        {"ULTRAPROD1", 1.23},
        {"ULTRAPROD2", 1.23},
        {"STORM_CRYSTAL", 3.28},
    }},
    {"R_EXPLORATION_2", {  -- stock weights add up to 360
        {"SHIP_INV_TOKEN", 24.48},
        {"WEAP_INV_TOKEN", 15.84},
        {"SUIT_INV_TOKEN", 10.8},
        {"ALIEN_INV_TOKEN", 3.6},
        {"FRIG_TOKEN", 24.48},
        {"SHIP_CORE_S", 2.16},
        {"SHIP_CORE_A", 4.68},
        {"ULTRAPROD1", 1.08},
        {"ULTRAPROD2", 1.08},
        {"STORM_CRYSTAL", 2.88},
    }},
    {"R_EXPLORATION_3", {  -- stock weights add up to 360
        {"SHIP_INV_TOKEN", 24.48},
        {"WEAP_INV_TOKEN", 15.84},
        {"SUIT_INV_TOKEN", 10.8},
        {"ALIEN_INV_TOKEN", 3.6},
        {"FRIG_TOKEN", 24.48},
        {"SHIP_CORE_S", 2.16},
        {"SHIP_CORE_A", 4.68},
        {"ULTRAPROD1", 1.08},
        {"ULTRAPROD2", 1.08},
        {"STORM_CRYSTAL", 2.88},
    }},
    {"R_MINING_0", {  -- stock weights add up to 500
        {"SHIP_INV_TOKEN", 34},
        {"WEAP_INV_TOKEN", 22},
        {"SUIT_INV_TOKEN", 15},
        {"ALIEN_INV_TOKEN", 5},
        {"FRIG_TOKEN", 34},
        {"SHIP_CORE_S", 3},
        {"SHIP_CORE_A", 6.5},
        {"ULTRAPROD1", 1.5},
        {"ULTRAPROD2", 1.5},
        {"STORM_CRYSTAL", 4},
    }},
    {"R_MINING_1", {  -- stock weights add up to 972
        {"WEAP_INV_TOKEN", 42.77},
        {"SUIT_INV_TOKEN", 29.16},
        {"ALIEN_INV_TOKEN", 9.72},
        {"SHIP_CORE_S", 5.83},
        {"SHIP_CORE_A", 12.64},
        {"ULTRAPROD1", 2.92},
        {"ULTRAPROD2", 2.92},
        {"STORM_CRYSTAL", 7.78},
    }},
    {"R_MINING_2", {  -- stock weights add up to 966
        {"WEAP_INV_TOKEN", 42.5},
        {"SUIT_INV_TOKEN", 28.98},
        {"ALIEN_INV_TOKEN", 9.66},
        {"FRIG_TOKEN", 65.69},
        {"SHIP_CORE_S", 5.8},
        {"SHIP_CORE_A", 12.56},
        {"ULTRAPROD1", 2.9},
        {"ULTRAPROD2", 2.9},
        {"STORM_CRYSTAL", 7.73},
    }},
    {"R_MINING_3", {  -- stock weights add up to 832
        {"WEAP_INV_TOKEN", 36.61},
        {"SUIT_INV_TOKEN", 24.96},
        {"ALIEN_INV_TOKEN", 8.32},
        {"SHIP_CORE_S", 4.99},
        {"SHIP_CORE_A", 10.82},
        {"ULTRAPROD1", 2.5},
        {"ULTRAPROD2", 2.5},
        {"STORM_CRYSTAL", 6.66},
    }},
}

-- One reward item, exactly as the tables write it (tab-indented to the
-- GcRewardTableItem depth). {ID}, {WEIGHT} and {AMOUNT} are filled in below.
local ITEM = [[
					<Property name="List" value="GcRewardTableItem">
						<Property name="PercentageChance" value="{WEIGHT}" />
						<Property name="LabelID" value="" />
						<Property name="Reward" value="GcRewardSpecificProduct">
							<Property name="GcRewardSpecificProduct">
								<Property name="Default" value="GcDefaultMissionProductEnum">
									<Property name="DefaultProductType" value="None" />
								</Property>
								<Property name="ID" value="{ID}" />
								<Property name="AmountMin" value="{AMOUNT}" />
								<Property name="AmountMax" value="{AMOUNT}" />
								<Property name="HideAmountInMessage" value="false" />
								<Property name="ForceSpecialMessage" value="false" />
								<Property name="HideInSeasonRewards" value="false" />
								<Property name="Silent" value="false" />
								<Property name="SeasonRewardListFormat" value="" />
								<Property name="RequiresTech" value="" />
							</Property>
						</Property>
					</Property>
]]

local function item(id, weight)
    local amount = 1
    if id == "STORM_CRYSTAL" then amount = 10 end
    return (ITEM:gsub("{ID}", id):gsub("{WEIGHT}", tostring(weight)):gsub("{AMOUNT}", tostring(amount)))
end

-- One ADD per list, inserted after its first item: the anchor is the list's
-- _id, then the first item under it.
local function adds(lists)
    local out = {}
    for _, l in ipairs(lists) do
        local xml = ""
        for _, w in ipairs(l[2]) do xml = xml .. item(w[1], w[2]) end
        table.insert(out, {
            ["SPECIAL_KEY_WORDS"] = {'_id="' .. l[1] .. '"', 'value="GcRewardTableItem" _index="0"'},
            -- Each item ends in a newline; the last one is dropped so the
            -- block ends on its closing tag.
            ["ADD"] = xml:sub(1, -2)
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

-- The adds come first, so the weighting reaches the added items too.
local function favoured()
    return concat(
        favour({"SHIP_INV_TOKEN"}, STORAGE_WEIGHT),
        favour({"WEAP_INV_TOKEN"}, MULTITOOL_WEIGHT),
        favour({"SUIT_INV_TOKEN"}, EXOSUIT_WEIGHT),
        favour({"ALIEN_INV_TOKEN"}, SPAWNING_SAC_WEIGHT),
        favour({"FRIG_TOKEN"}, FRIGATE_MODULE_WEIGHT),
        favour({"SHIP_CORE_S", "SHIP_CORE_A"}, REACTOR_WEIGHT),
        favour({"ULTRAPROD1", "ULTRAPROD2"}, ULTRAPROD_WEIGHT),
        favour({"STORM_CRYSTAL"}, STORM_CRYSTAL_WEIGHT))
end
local reward = concat(adds(NEXUS_ADDS), favoured())
local expedition = concat(adds(EXPEDITION_ADDS), favoured())

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

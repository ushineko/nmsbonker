-- @tweak name="Nexus mission rewards" group="Missions"
-- @desc Multiplies what Nexus missions pay, on top of the global reward tweaks:
-- @desc the item lists, and the units, nanites and quicksilver that come with
-- @desc them. Nothing outside the seven Nexus reward entries is touched.
-- @param ITEM_MULT label="Item amount multiplier" min=1 max=100 step=1 default=5
-- @param UNITS_MULT label="Units multiplier" min=1 max=100 step=1 default=5
-- @param NANITE_MULT label="Nanites multiplier" min=1 max=100 step=1 default=1
-- @param QS_MULT label="Quicksilver multiplier" min=1 max=100 step=1 default=5
-- @param ITEM_CAP label="Largest item amount" min=0 max=1000000 step=1000 default=50000
-- @param UNITS_CAP label="Largest units reward" min=0 max=2000000000 step=1000000 default=50000000
-- @param NANITES_CAP label="Largest nanites reward" min=0 max=10000000 step=10000 default=250000
-- @param QS_CAP label="Largest quicksilver reward" min=0 max=10000000 step=10000 default=100000
-- Nexus missions pay from seven entries of REWARDTABLE, all in the
-- MissionBoardTable list. Stock amounts are small (one of an item, a few hundred
-- nanites), so a global multiplier leaves them feeling thin next to everything
-- else it lifts. This multiplies those entries again, by entry, so nothing else
-- in the table moves. It runs after the global tweaks in build order, so the
-- factors compound: 1 item x10 (chest and loot) x5 (here) = 50.
-- Quicksilver is the currency the game calls "Specials"; no other tweak touches it.
ITEM_MULT   = 5
UNITS_MULT  = 5
NANITE_MULT = 1
QS_MULT     = 5
ITEM_CAP    = 50000     -- ceilings on the final amount, whatever ran before. 0 = none.
UNITS_CAP   = 50000000
NANITES_CAP = 250000
QS_CAP      = 100000

local ITEM_ENTRIES     = { "R_NEXUS_MED", "R_NEXUS_MEGA" }
local CURRENCY_ENTRIES = { "R_NEXUS_MED_C", "R_NEXUS_MEGA_C", "R_NEXUS_CASH", "R_NEXUS_QS", "R_NEXUS_QS_PQ" }
local ITEM_WRAPPERS    = { "GcRewardSpecificProduct", "GcRewardSpecificProductFromList", "GcRewardSpecificSubstance" }

local changes = {}
for _, entry in ipairs(ITEM_ENTRIES) do
    for _, wrapper in ipairs(ITEM_WRAPPERS) do
        table.insert(changes, { ["WRAPPER_MULT"] = { ["WRAPPER"] = wrapper, ["MULT"] = ITEM_MULT, ["ENTRY"] = entry }, ["CAP"] = ITEM_CAP })
    end
end
for _, entry in ipairs(CURRENCY_ENTRIES) do
    table.insert(changes, { ["CURRENCY_MULT"] = { ["CURRENCY"] = "Units",    ["MULT"] = UNITS_MULT,  ["ENTRY"] = entry }, ["CAP"] = UNITS_CAP })
    table.insert(changes, { ["CURRENCY_MULT"] = { ["CURRENCY"] = "Nanites",  ["MULT"] = NANITE_MULT, ["ENTRY"] = entry }, ["CAP"] = NANITES_CAP })
    table.insert(changes, { ["CURRENCY_MULT"] = { ["CURRENCY"] = "Specials", ["MULT"] = QS_MULT,     ["ENTRY"] = entry }, ["CAP"] = QS_CAP })
end

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "NexusRewards.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\REWARDTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] = changes
                }
            }
        }
    }
}

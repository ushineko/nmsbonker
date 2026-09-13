-- @tweak name="Mission board rewards" group="Missions"
-- @desc Multiplies what station mission board and corvette missions pay, on top
-- @desc of the global reward tweaks: the item lists and the units and nanites.
-- @desc Ships neutral; only the eight mission board entries are touched.
-- @param ITEM_MULT label="Item amount multiplier" min=1 max=100 step=1 default=1
-- @param UNITS_MULT label="Units multiplier" min=1 max=100 step=1 default=1
-- @param NANITE_MULT label="Nanites multiplier" min=1 max=100 step=1 default=1
-- @param ITEM_CAP label="Largest item amount" min=0 max=1000000 step=1000 default=50000
-- @param UNITS_CAP label="Largest units reward" min=0 max=2000000000 step=1000000 default=50000000
-- @param NANITES_CAP label="Largest nanites reward" min=0 max=10000000 step=10000 default=250000
-- The faction and guild agents at a space station hand out missions that pay
-- from R_MB_LOW..R_MB_MEGA; corvette missions pay from the parallel R_CV_*
-- entries. Each entry mixes item rewards and money, so items and currency get
-- their own multipliers. Multiplied by entry, after the global tweaks, so the
-- rest of the table is unchanged. Nexus missions have their own tweak.
ITEM_MULT   = 1
UNITS_MULT  = 1
NANITE_MULT = 1
ITEM_CAP    = 50000     -- ceilings on the final amount, whatever ran before. 0 = none.
UNITS_CAP   = 50000000
NANITES_CAP = 250000

local ENTRIES = { "R_MB_LOW", "R_MB_MED", "R_MB_HIGH", "R_MB_MEGA",
                  "R_CV_LOW", "R_CV_MED", "R_CV_HIGH", "R_CV_MEGA" }
local ITEM_WRAPPERS = { "GcRewardSpecificProduct", "GcRewardSpecificProductFromList", "GcRewardSpecificSubstance" }

local changes = {}
for _, entry in ipairs(ENTRIES) do
    for _, wrapper in ipairs(ITEM_WRAPPERS) do
        table.insert(changes, { ["WRAPPER_MULT"] = { ["WRAPPER"] = wrapper, ["MULT"] = ITEM_MULT, ["ENTRY"] = entry }, ["CAP"] = ITEM_CAP })
    end
    table.insert(changes, { ["CURRENCY_MULT"] = { ["CURRENCY"] = "Units",   ["MULT"] = UNITS_MULT,  ["ENTRY"] = entry }, ["CAP"] = UNITS_CAP })
    table.insert(changes, { ["CURRENCY_MULT"] = { ["CURRENCY"] = "Nanites", ["MULT"] = NANITE_MULT, ["ENTRY"] = entry }, ["CAP"] = NANITES_CAP })
end

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "MissionBoardRewards.pak",
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

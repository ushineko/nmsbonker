-- @tweak name="Big stacks" group="Inventory"
-- @desc Raises the inventory stack limits, for every difficulty tier and every
-- @desc inventory. Absolute caps rather than a multiplier: products start at
-- @desc five to twenty and the game uses hard limits. Also bounds what an
-- @desc antimatter harvester may hoard, which a raised stack limit otherwise
-- @desc turns into a full stack per harvester.
-- @param SUBSTANCE label="Substance stack limit" min=1 max=9999999 step=1000 default=999999
-- @param PRODUCT label="Product stack limit" min=1 max=9999999 step=1000 default=99999
-- @param ANTIMATTER_HARVESTER_CAP label="Antimatter per harvester" min=0 max=99999 step=1 default=20
-- Big inventory stacks: raise the difficulty stack caps AND the per-inventory
-- max stack sizes across ALL difficulty tiers, so heavy looting/mining doesn't
-- flood the inventory. Absolute caps (not a small multiplier) since products
-- start tiny (5-20) and NMS uses hard limits.
SUBSTANCE = 999999
PRODUCT   = 99999

-- Antimatter harvesters hold "a full stack" (MaxCapacity -1) by default, which
-- with the raised product limit above means 99,999 antimatter per harvester --
-- enough that the resource stops being a resource. Set an absolute figure
-- instead. 0 leaves the game's own -1 alone.
ANTIMATTER_HARVESTER_CAP = 20

local mbin_changes =
{
    {
        ["MBIN_FILE_SOURCE"] = "METADATA\GAMESTATE\DIFFICULTYCONFIG.MBIN",
        ["EXML_CHANGE_TABLE"] =
        {
            -- difficulty tier caps (3 tiers) -> raise both
            {
                ["PRECEDING_KEY_WORDS"] = "",
                ["REPLACE_TYPE"]        = "ALL",
                ["VALUE_CHANGE_TABLE"]  = { {"SubstanceStackLimit", SUBSTANCE}, {"ProductStackLimit", PRODUCT} }
            },
            -- per-inventory substance max, every MaxSubstanceStackSizes block
            {
                ["SPECIAL_KEY_WORDS"] = {"MaxSubstanceStackSizes"},
                ["REPLACE_TYPE"]      = "ALL",
                ["VALUE_CHANGE_TABLE"]= {
                    {"Default",SUBSTANCE},{"Personal",SUBSTANCE},{"Ship",SUBSTANCE},{"Freighter",SUBSTANCE},
                    {"Vehicle",SUBSTANCE},{"Chest",SUBSTANCE},{"BaseCapsule",SUBSTANCE},
                    {"MaintenanceObject",SUBSTANCE},{"UIPopup",SUBSTANCE}
                }
            },
            -- per-inventory product max, every MaxProductStackSizes block
            {
                ["SPECIAL_KEY_WORDS"] = {"MaxProductStackSizes"},
                ["REPLACE_TYPE"]      = "ALL",
                ["VALUE_CHANGE_TABLE"]= {
                    {"Default",PRODUCT},{"Personal",PRODUCT},{"Ship",PRODUCT},{"Freighter",PRODUCT},
                    {"Vehicle",PRODUCT},{"Chest",PRODUCT},{"BaseCapsule",PRODUCT},{"MaintenanceObject",PRODUCT}
                }
            }
        }
    }
}

-- The harvester entity has TWO GcMaintenanceElement blocks -- MAINT_FUEL4, which
-- is the fuel slot, and ANTIMATTER, which is the one that fills up -- and they
-- carry a MaxCapacity each. Anchoring on GcMaintenanceElement alone would take
-- the first, which is the fuel slot and not what anyone means by "how much
-- antimatter". Both keywords together anchor on the ANTIMATTER block and the
-- value change is then scoped to that block, so exactly one MaxCapacity moves.
if ANTIMATTER_HARVESTER_CAP > 0 then
  table.insert(mbin_changes,
    {
        ["MBIN_FILE_SOURCE"] = "MODELS\PLANETS\BIOMES\COMMON\BUILDINGS\PARTS\BUILDABLEPARTS\TECH\ANTIMATTERHARVESTER\ENTITIES\ANTIMATTERHARVESTER.ENTITY.MBIN",
        ["EXML_CHANGE_TABLE"] =
        {
            {
                ["SPECIAL_KEY_WORDS"]  = {"GcMaintenanceElement", "ANTIMATTER"},
                ["VALUE_CHANGE_TABLE"] = { {"MaxCapacity", ANTIMATTER_HARVESTER_CAP} }
            }
        }
    })
end

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "BigStacks.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] = mbin_changes
        }
    }
}

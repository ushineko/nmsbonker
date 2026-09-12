-- @tweak name="Scan value" group="Exploration"
-- @desc Multiplies the base analysis and discovery payouts for creatures,
-- @desc plants, minerals, planets and systems, and sets a flat value for a
-- @desc starship scan, which stock pays nothing for.
-- @param SCAN_MULTIPLIER label="Scan payout multiplier" min=1 max=200 step=1 default=50
-- @param SHIP_FLAT label="Starship scan value" min=0 max=1000000 step=1000 default=25000
-- @param SCAN_CAP label="Largest scan payout" min=0 max=100000000 step=100000 default=5000000
-- Multiply the BASE scan/analysis + discovery unit reward (before scanner
-- upgrades) to make exploration a worthwhile income source.
--   Animal/Flora/Mineral: OnScan (the +units when you analyse) x50.
--   Planet/SolarSystem:   Record (their payout is the discovery-log value) x50.
--   Starship:             stock value is 0 (no scan reward exists), so SET a
--                         flat value; may be inert in-game if no ship scan
--                         reward event fires.
-- All in GcDiscoveryWorth blocks of METADATA/REALITY/DEFAULTREALITY.MBIN.
SCAN_MULTIPLIER = 50
SHIP_FLAT       = 25000
SCAN_CAP        = 5000000  -- ceiling on a multiplied payout. 0 = no ceiling. The two
                           -- Starship values are set rather than multiplied, so the
                           -- ceiling does not apply to them: SHIP_FLAT is the value.

local function onscan_x(cat)
  return { ["SPECIAL_KEY_WORDS"]={cat,"OnScan"}, ["MATH_OPERATION"]="*", ["CAP"]=SCAN_CAP,
           ["VALUE_CHANGE_TABLE"]={{"Common",SCAN_MULTIPLIER},{"Uncommon",SCAN_MULTIPLIER},{"Rare",SCAN_MULTIPLIER}} }
end
local function record_x(cat)
  return { ["SPECIAL_KEY_WORDS"]={cat,"Record"}, ["MATH_OPERATION"]="*", ["CAP"]=SCAN_CAP,
           ["VALUE_CHANGE_TABLE"]={{"Common",SCAN_MULTIPLIER},{"Uncommon",SCAN_MULTIPLIER},{"Rare",SCAN_MULTIPLIER}} }
end
local function record_set(cat,v)
  return { ["SPECIAL_KEY_WORDS"]={cat,"Record"},
           ["VALUE_CHANGE_TABLE"]={{"Common",v},{"Uncommon",v},{"Rare",v}} }
end
local function onscan_set(cat,v)
  return { ["SPECIAL_KEY_WORDS"]={cat,"OnScan"},
           ["VALUE_CHANGE_TABLE"]={{"Common",v},{"Uncommon",v},{"Rare",v}} }
end

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ScanValue50x.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\DEFAULTREALITY.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        onscan_x("Animal"), onscan_x("Flora"), onscan_x("Mineral"),
                        record_x("Planet"), record_x("SolarSystem"),
                        record_set("Starship", SHIP_FLAT), onscan_set("Starship", SHIP_FLAT)
                    }
                }
            }
        }
    }
}

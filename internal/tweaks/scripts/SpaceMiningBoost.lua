-- @tweak name="Space mining" group="Mining"
-- @desc Multiplies the resources an asteroid holds and raises the chance that
-- @desc a shot at one yields anything at all.
-- @param AST_MULT label="Asteroid resource multiplier" min=1 max=100 step=1 default=20
-- @param VOXEL_CHANCE label="Asteroid yield chance" min=0 max=1 step=0.05 default=1.0 kind=float
-- Boost asteroid mining yield at the real source: GCSOLARGENERATIONGLOBALS holds
-- the per-asteroid resource amounts. Common = tritium/common (ASTEROID1/2),
-- Rare = precious (gold/platinum/silver, ASTEROID3). Multiply those x20.
-- Also raise VoxelAsteroidResourceChance so nearly every hit yields.
-- (AsteroidResourceReducer left at stock — it is not the amount lever.)
AST_MULT     = 20
VOXEL_CHANCE = 1.0

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "SpaceMiningBoost.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "GCSOLARGENERATIONGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["PRECEDING_KEY_WORDS"] = "",
                            ["MATH_OPERATION"]      = "*",
                            ["VALUE_CHANGE_TABLE"]  = {
                                {"Common Asteroid Min Resources", AST_MULT},
                                {"Common Asteroid Max Resources", AST_MULT},
                                {"Rare Asteroid Min Resources",   AST_MULT},
                                {"Rare Asteroid Max Resources",   AST_MULT}
                            }
                        }
                    }
                },
                {
                    ["MBIN_FILE_SOURCE"] = "GCGAMEPLAYGLOBALS.GLOBAL.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["PRECEDING_KEY_WORDS"] = "",
                            ["VALUE_CHANGE_TABLE"]  = { {"VoxelAsteroidResourceChance", VOXEL_CHANCE} }
                        }
                    }
                }
            }
        }
    }
}

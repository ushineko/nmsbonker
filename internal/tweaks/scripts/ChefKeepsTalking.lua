-- @tweak name="Chef keeps talking" group="Interface"
-- @desc The Nexus chef's conversation stays open after each answer, so several
-- @desc dishes can be asked about without walking away and back. After
-- @desc JustRuthless's "Keep Talking Chef".

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ChefKeepsTalking.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\NMS_DIALOG_GCALIENPUZZLETABLE.MBIN",
                    -- Each judging option is found by what it costs; its KeepOpen
                    -- is a sibling of the cost, so the scope is the option one
                    -- level up. Anchored on the cost line itself, the edit would
                    -- find nothing to change.
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"C_NEXUSCHEF1"},
                            ["SECTION_UP"]         = 1,
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"KeepOpen", "true"} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"C_NEXUSCHEF2"},
                            ["SECTION_UP"]         = 1,
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"KeepOpen", "true"} }
                        },
                        {
                            ["SPECIAL_KEY_WORDS"]  = {"C_NEXUSCHEF3"},
                            ["SECTION_UP"]         = 1,
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"KeepOpen", "true"} }
                        }
                    }
                }
            }
        }
    }
}

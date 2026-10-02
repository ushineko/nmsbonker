-- @tweak name="Instant text" group="Interface"
-- @desc Dialogue and interaction text appears at once rather than being typed
-- @desc out a letter at a time: every per-character and punctuation delay is
-- @desc set to this. After NooBzPoWaH's "InstantTextDisplay"; delays only, no
-- @desc added entries.
-- @param TEXT_DELAY label="Delay per character, seconds" min=0 max=0.05 step=0.001 default=0.003 kind=float
TEXT_DELAY = 0.003  -- stock 0.02 per character, up to 0.5 after punctuation

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "InstantText.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\UI\SPECIALTEXTPUNCTUATIONDELAYDATA.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["REPLACE_TYPE"]       = "ALL",
                            ["VALUE_CHANGE_TABLE"] = { {"Delay", TEXT_DELAY}, {"DefaultDelay", TEXT_DELAY} }
                        }
                    }
                }
            }
        }
    }
}

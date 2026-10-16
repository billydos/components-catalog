package domain

import (
	"maps"
	"slices"

	"github.com/billydos/components-catalog/internal/i18n"
)

// MsgID — стабильный идентификатор сообщения каталога (этап 8.3, D9):
// латиница snake_case, реестр констант ниже. Форматы сообщений — бандлы
// internal/i18n (en — канонический и обязательный, ru — полная локаль);
// Message ошибки — канонический en-рендер, локализованный рендер по
// MsgID+Args выполняют транспорты (REST — Accept-Language, CLI --lang).

// MsgID реестра сообщений. Расширение — новой константой здесь, строкой
// в registry и форматами en/ru в internal/i18n тем же изменением
// (полнота закреплена тестом полноты каталога).
type MsgID string

const (
	MsgSchemaVersionMismatch  MsgID = "schema_version_mismatch"
	MsgDatabaseNotInitialized MsgID = "database_not_initialized"
	MsgKindNotSupported       MsgID = "kind_not_supported"
	MsgGostSubclassDictSync   MsgID = "gost_subclass_dict_sync"
	MsgKindAmbiguous          MsgID = "kind_ambiguous"
	MsgKindAmbiguousMismatch  MsgID = "kind_ambiguous_mismatch"
	MsgKindRequired           MsgID = "kind_required"

	MsgEmptyDesignation MsgID = "empty_designation"

	MsgScannerExpected   MsgID = "scanner_expected"
	MsgScannerUnexpected MsgID = "scanner_unexpected"
	MsgScannerToken      MsgID = "scanner_token"
	MsgScannerEof        MsgID = "scanner_eof"

	MsgKindUnknown     MsgID = "kind_unknown"
	MsgSystemUnknown   MsgID = "system_unknown"
	MsgUnknownMaterial MsgID = "unknown_material"
	MsgUnknownSubclass MsgID = "unknown_subclass"
	MsgUnknownAdjust   MsgID = "unknown_adjustment"
	MsgUnknownCategory MsgID = "unknown_category"
	MsgPowerSuffix     MsgID = "power_suffix"
	MsgDevNumberZeros  MsgID = "dev_number_leading_zero"
	MsgDevNumberDigits MsgID = "dev_number_digits"

	MsgCanonicalAlphabetMix MsgID = "canonical_alphabet_mix"
	MsgCanonicalBadRune     MsgID = "canonical_bad_rune"
	MsgCanonicalNoLetters   MsgID = "canonical_no_letters"
	MsgKindMismatch         MsgID = "kind_mismatch"
	MsgSystemMismatch       MsgID = "system_mismatch"
	MsgAutodetectFailed     MsgID = "autodetect_failed"
	MsgCapGroupUnknown      MsgID = "cap_group_unknown"

	// Фрагменты грамматик «ожидалось: …» — аргументы scanner-сообщений.
	MsgExpectGostSubclass      MsgID = "expect_gost_subclass"
	MsgExpectGostOptoFunction  MsgID = "expect_gost_opto_function"
	MsgExpectGostDevNumberFull MsgID = "expect_gost_dev_number_full"
	MsgExpectGostDevNumber     MsgID = "expect_gost_dev_number"
	MsgExpectGostMaterial      MsgID = "expect_gost_material"
	MsgExpectDevNumber         MsgID = "expect_dev_number"
	MsgExpectGostFeatureAndDev MsgID = "expect_gost_feature_and_dev"
	MsgExpectGostFeatureDigit  MsgID = "expect_gost_feature_digit"
	MsgExpectGostDevOne        MsgID = "expect_gost_dev_one"
	MsgExpectGostDevOneDigit   MsgID = "expect_gost_dev_one_digit"
	MsgExpectGostFeatureDevRun MsgID = "expect_gost_feature_dev_run"
	MsgExpectGostModification  MsgID = "expect_gost_modification"
	MsgExpectGostChip          MsgID = "expect_gost_chip"
	MsgExpectGostMakerCode     MsgID = "expect_gost_maker_code"
	MsgExpectJedecJunctions    MsgID = "expect_jedec_junctions"
	MsgExpectJedecLetter       MsgID = "expect_jedec_letter"
	MsgExpectJedecNumber       MsgID = "expect_jedec_number"
	MsgExpectJedecNumberNoZero MsgID = "expect_jedec_number_nonzero"
	MsgExpectJisJunctions      MsgID = "expect_jis_junctions"
	MsgExpectJisS              MsgID = "expect_jis_s"
	MsgExpectJisClassLetter    MsgID = "expect_jis_class_letter"
	MsgExpectJisClassRequired  MsgID = "expect_jis_class_required"
	MsgExpectJisNumber         MsgID = "expect_jis_number"
	MsgExpectJisSuffix         MsgID = "expect_jis_suffix"
	MsgExpectJisShortForm      MsgID = "expect_jis_short_form"
	MsgExpectJisShortNumber    MsgID = "expect_jis_short_number"
	MsgExpectProMaterial       MsgID = "expect_pro_material"
	MsgExpectProClassLetter    MsgID = "expect_pro_class_letter"
	MsgExpectProClassTable     MsgID = "expect_pro_class_table"
	MsgExpectProNumber         MsgID = "expect_pro_number"
	MsgExpectProConsumerNumber MsgID = "expect_pro_consumer_number"
	MsgExpectProAnyNumber      MsgID = "expect_pro_any_number"
	MsgExpectProSuffix         MsgID = "expect_pro_suffix"
	MsgExpectResFamilyCS       MsgID = "expect_res_family_cs"
	MsgExpectResFamilyR        MsgID = "expect_res_family_r"
	MsgExpectHyphen            MsgID = "expect_hyphen"
	MsgExpectCapGroup          MsgID = "expect_cap_group"
	MsgExpectCapPrefix         MsgID = "expect_cap_prefix"
	MsgExpectCapSubclassGroup  MsgID = "expect_cap_subclass_group"
	MsgExpectResMaterialGroup6 MsgID = "expect_res_material_group_6"
	MsgExpectResMaterialGroup2 MsgID = "expect_res_material_group_2"

	// Сообщения движка валидации и метасхемы (internal/catalog).
	MsgEngineKindMissing              MsgID = "engine_kind_missing"
	MsgEngineKindNoVariants           MsgID = "engine_kind_no_variants"
	MsgEngineSystemNotApplicable      MsgID = "engine_system_not_applicable"
	MsgEngineAttrUnknown              MsgID = "engine_attr_unknown"
	MsgEngineAttrInactive             MsgID = "engine_attr_inactive"
	MsgEngineAttrNotApplicable        MsgID = "engine_attr_not_applicable"
	MsgEngineAttrTextExpected         MsgID = "engine_attr_text_expected"
	MsgEngineAttrTextEmpty            MsgID = "engine_attr_text_empty"
	MsgEngineAttrEnumExpected         MsgID = "engine_attr_enum_expected"
	MsgEngineAttrEnumInvalid          MsgID = "engine_attr_enum_invalid"
	MsgEngineAttrBoolExpected         MsgID = "engine_attr_bool_expected"
	MsgEngineAttrIntExpected          MsgID = "engine_attr_int_expected"
	MsgEngineAttrNumExpected          MsgID = "engine_attr_num_expected"
	MsgEngineAttrWhole                MsgID = "engine_attr_whole"
	MsgEngineAttrPositive             MsgID = "engine_attr_positive"
	MsgEngineParamUnknown             MsgID = "engine_param_unknown"
	MsgEngineParamInactive            MsgID = "engine_param_inactive"
	MsgEngineParamNotApplicable       MsgID = "engine_param_not_applicable"
	MsgEngineParamWrongSection        MsgID = "engine_param_wrong_section"
	MsgEngineParamDuplicate           MsgID = "engine_param_duplicate"
	MsgEngineParamTextEmpty           MsgID = "engine_param_text_empty"
	MsgValueKeyRequired               MsgID = "value_key_required"
	MsgValueKeyForbidden              MsgID = "value_key_forbidden"
	MsgValueKeysPairRequired          MsgID = "value_keys_pair_required"
	MsgValueEnumInvalid               MsgID = "value_enum_invalid"
	MsgValueKeyPositive               MsgID = "value_key_positive"
	MsgValueKeyCeiling                MsgID = "value_key_ceiling"
	MsgValueMinMax                    MsgID = "value_min_max"
	MsgCondDuplicate                  MsgID = "cond_duplicate"
	MsgCondUnknown                    MsgID = "cond_unknown"
	MsgCondPositive                   MsgID = "cond_positive"
	MsgCondNotAllowed                 MsgID = "cond_not_allowed"
	MsgCondFixed                      MsgID = "cond_fixed"
	MsgCondSetMismatch                MsgID = "cond_set_mismatch"
	MsgRuleYearRange                  MsgID = "rule_year_range_value"
	MsgRuleYearOrder                  MsgID = "rule_year_order"
	MsgRuleTempPair                   MsgID = "rule_temp_pair_order"
	MsgRuleCapCylIncomplete           MsgID = "rule_cap_cyl_incomplete"
	MsgRuleCapFormMix                 MsgID = "rule_cap_form_mix"
	MsgRuleCapRectIncomplete          MsgID = "rule_cap_rect_incomplete"
	MsgRuleVariantVnomMissing         MsgID = "rule_variant_vnom_missing"
	MsgRuleVariantCnomMissing         MsgID = "rule_variant_cnom_missing"
	MsgRuleVariantVnomDup             MsgID = "rule_variant_vnom_dup"
	MsgRuleVariantLabelDup            MsgID = "rule_variant_label_dup"
	MsgRuleVariantPnomMissing         MsgID = "rule_variant_pnom_missing"
	MsgRuleVariantPnomDup             MsgID = "rule_variant_pnom_dup"
	MsgRuleRelayVariantRcoilMissing   MsgID = "rule_variant_rcoil_missing"
	MsgRuleRelayVariantNominalMissing MsgID = "rule_variant_nominal_missing"
	MsgMetaSectionNoCode              MsgID = "meta_section_no_code"
	MsgMetaSectionDupCode             MsgID = "meta_section_dup_code"
	MsgMetaRuleUnknownSection         MsgID = "meta_rule_unknown_section"
	MsgMetaParamValueType             MsgID = "meta_param_value_type"
	MsgMetaParamRuleUnknown           MsgID = "meta_param_rule_unknown"
	MsgMetaCondModeUnknown            MsgID = "meta_cond_mode_unknown"
	MsgMetaAttrValueType              MsgID = "meta_attr_value_type"
	MsgMetaAttrRuleUnknown            MsgID = "meta_attr_rule_unknown"
	MsgMetaSystemRefMissing           MsgID = "meta_system_ref_missing"
	MsgMetaSystemKindMissing          MsgID = "meta_system_kind_missing"
	MsgMetaCondUnitMissing            MsgID = "meta_cond_unit_missing"
	MsgMetaCondCodeReserved           MsgID = "meta_cond_code_reserved"
	MsgMetaGroupNoSection             MsgID = "meta_group_no_section"
	MsgMetaGroupSortNegative          MsgID = "meta_group_sort_negative"
	MsgMetaGroupSectionDup            MsgID = "meta_group_section_dup"
	MsgMetaGroupSectionReserved       MsgID = "meta_group_section_reserved"
	MsgMetaRuleUnknown                MsgID = "meta_rule_unknown"
	MsgMetaKindRuleKindMissing        MsgID = "meta_kind_rule_kind_missing"
	MsgMetaKindRuleUnknown            MsgID = "meta_kind_rule_unknown"
	MsgMetaKindRuleRowMissing         MsgID = "meta_kind_rule_row_missing"
	MsgMetaKindRuleNotDevice          MsgID = "meta_kind_rule_not_device"
	MsgMetaParamGroupMissing          MsgID = "meta_param_group_missing"
	MsgMetaParamUnitMissing           MsgID = "meta_param_unit_missing"
	MsgMetaParamUnitForbidden         MsgID = "meta_param_unit_forbidden"
	MsgMetaParamEnumRequired          MsgID = "meta_param_enum_required"
	MsgMetaParamEnumOnly              MsgID = "meta_param_enum_only"
	MsgMetaParamCeiling               MsgID = "meta_param_ceiling"
	MsgMetaParamSortNegative          MsgID = "meta_param_sort_negative"
	MsgMetaParamKindMissing           MsgID = "meta_param_kind_missing"
	MsgMetaParamRuleRowMissing        MsgID = "meta_param_rule_row_missing"
	MsgMetaParamRuleWrongLevel        MsgID = "meta_param_rule_wrong_level"
	MsgMetaCondSetEmpty               MsgID = "meta_cond_set_empty"
	MsgMetaCondMissing                MsgID = "meta_cond_missing"
	MsgMetaCondFixedOptional          MsgID = "meta_cond_fixed_optional"
	MsgMetaAttrUnitMissing            MsgID = "meta_attr_unit_missing"
	MsgMetaAttrUnitForbidden          MsgID = "meta_attr_unit_forbidden"
	MsgMetaAttrEnumRequired           MsgID = "meta_attr_enum_required"
	MsgMetaAttrEnumOnly               MsgID = "meta_attr_enum_only"
	MsgMetaAttrSortNegative           MsgID = "meta_attr_sort_negative"
	MsgMetaAttrKindMissing            MsgID = "meta_attr_kind_missing"
	MsgMetaAttrRuleRowMissing         MsgID = "meta_attr_rule_row_missing"
	MsgMetaAttrRuleWrongLevel         MsgID = "meta_attr_rule_wrong_level"

	// Сообщения формата наполнения (internal/importer).
	MsgImportRootNotObject            MsgID = "import_root_not_object"
	MsgImportRootUnknownKey           MsgID = "import_root_unknown_key"
	MsgImportRootSectionArray         MsgID = "import_root_section_array"
	MsgImportRecordEmptyName          MsgID = "import_record_empty_name"
	MsgImportRecordBadValue           MsgID = "import_record_bad_value"
	MsgImportRecordNameRequired       MsgID = "import_record_name_required"
	MsgImportRecordNameString         MsgID = "import_record_name_string"
	MsgImportRecordUnknownField       MsgID = "import_record_unknown_field"
	MsgImportRecordSystemString       MsgID = "import_record_system_string"
	MsgImportRecordSystemUnknown      MsgID = "import_record_system_unknown"
	MsgImportFieldsObject             MsgID = "import_fields_object"
	MsgImportFieldUnknown             MsgID = "import_field_unknown"
	MsgImportFieldString              MsgID = "import_field_string"
	MsgImportFieldNumber              MsgID = "import_field_number"
	MsgImportSectionArray             MsgID = "import_section_array"
	MsgImportManufacturersArray       MsgID = "import_manufacturers_array"
	MsgImportManufacturerItem         MsgID = "import_manufacturer_item"
	MsgImportVariantsArray            MsgID = "import_variants_array"
	MsgImportAnalogsArray             MsgID = "import_analogs_array"
	MsgImportAttributesObject         MsgID = "import_attributes_object"
	MsgImportAttrNumber               MsgID = "import_attr_number"
	MsgImportAttrBadValue             MsgID = "import_attr_bad_value"
	MsgImportWhereSection             MsgID = "import_where_section"
	MsgImportValueObject              MsgID = "import_value_object"
	MsgImportValueParameterString     MsgID = "import_value_parameter_string"
	MsgImportValueTextString          MsgID = "import_value_text_string"
	MsgImportValueKeyNumber           MsgID = "import_value_key_number"
	MsgImportValueUnknownKey          MsgID = "import_value_unknown_key"
	MsgImportValueParameterRequired   MsgID = "import_value_parameter_required"
	MsgImportVariantLabelString       MsgID = "import_variant_label_string"
	MsgImportVariantUnknownField      MsgID = "import_variant_unknown_field"
	MsgImportVariantSectionArray      MsgID = "import_variant_section_array"
	MsgImportAnalogNameRequired       MsgID = "import_analog_name_required"
	MsgImportAnalogNoteString         MsgID = "import_analog_note_string"
	MsgImportAnalogUnknownField       MsgID = "import_analog_unknown_field"
	MsgImportAnalogItem               MsgID = "import_analog_item"
	MsgImportNdjsonLineObject         MsgID = "import_ndjson_line_object"
	MsgImportNdjsonCatalogOrder       MsgID = "import_ndjson_catalog_order"
	MsgImportNdjsonClassKey           MsgID = "import_ndjson_class_key"
	MsgImportNdjsonWrapperSingle      MsgID = "import_ndjson_wrapper_single"
	MsgImportCatalogObject            MsgID = "import_catalog_object"
	MsgImportCatalogSubsection        MsgID = "import_catalog_subsection"
	MsgImportCatalogRowsArray         MsgID = "import_catalog_rows_array"
	MsgImportCatalogRowObject         MsgID = "import_catalog_row_object"
	MsgImportCatalogUnknownField      MsgID = "import_catalog_unknown_field"
	MsgImportCatalogFieldRequired     MsgID = "import_catalog_field_required"
	MsgImportCatalogFieldString       MsgID = "import_catalog_field_string"
	MsgImportCatalogFieldNumber       MsgID = "import_catalog_field_number"
	MsgImportCatalogFieldInteger      MsgID = "import_catalog_field_integer"
	MsgImportCatalogFieldBool         MsgID = "import_catalog_field_bool"
	MsgImportCatalogFieldStrings      MsgID = "import_catalog_field_strings"
	MsgImportCatalogItemString        MsgID = "import_catalog_item_string"
	MsgImportCatalogParamActiveBool   MsgID = "import_catalog_param_active_bool"
	MsgImportCatalogAttrActiveBool    MsgID = "import_catalog_attr_active_bool"
	MsgImportCatalogCondsetsArray     MsgID = "import_catalog_condsets_array"
	MsgImportCatalogCondsetObject     MsgID = "import_catalog_condset_object"
	MsgImportCatalogCondsetItems      MsgID = "import_catalog_condset_items"
	MsgImportCatalogCondsetItemObject MsgID = "import_catalog_condset_item_object"
	MsgImportCatalogCondsetEmpty      MsgID = "import_catalog_condset_empty"
	MsgImportCatalogRow               MsgID = "import_catalog_row"
	MsgImportCatalogRowCode           MsgID = "import_catalog_row_code"
	MsgImportFormatUnknown            MsgID = "import_format_unknown"
	MsgImportFormatByExt              MsgID = "import_format_by_ext"
	MsgImportFormatLineOnly           MsgID = "import_format_line_only"
	MsgImportYamlNonFinite            MsgID = "import_yaml_non_finite"
	MsgImportYamlMultiDoc             MsgID = "import_yaml_multi_doc"
	MsgImportYamlAliasCycle           MsgID = "import_yaml_alias_cycle"
	MsgImportYamlAliasUnknown         MsgID = "import_yaml_alias_unknown"
	MsgImportYamlKeyInvalid           MsgID = "import_yaml_key_invalid"
	MsgImportYamlNodeUnsupported      MsgID = "import_yaml_node_unsupported"
	MsgImportNdjsonLineError          MsgID = "import_ndjson_line_error"
	MsgImportSyntaxError              MsgID = "import_syntax_error"
	MsgImportCatalogRecordsMixed      MsgID = "import_catalog_records_mixed"
	MsgImportDuplicateKey             MsgID = "import_duplicate_key"
	MsgImportUnexpectedToken          MsgID = "import_unexpected_token"
	MsgImportIssueRecordLine          MsgID = "import_issue_record_line"
	MsgImportIssueRecord              MsgID = "import_issue_record"
	MsgImportIssueLine                MsgID = "import_issue_line"
	MsgImportIssueNo                  MsgID = "import_issue_no"
	MsgImportValueKindNumber          MsgID = "import_value_kind_number"
	MsgImportValueKindString          MsgID = "import_value_kind_string"
	MsgImportValueKindArray           MsgID = "import_value_kind_array"
	MsgImportValueKindObject          MsgID = "import_value_kind_object"
	MsgImportValueKindBool            MsgID = "import_value_kind_bool"
	MsgImportValueKindOther           MsgID = "import_value_kind_other"

	// Итог прогона и позиционные префиксы исполнений/аналогов.
	MsgImportSummary          MsgID = "import_summary"
	MsgImportSummaryRejected  MsgID = "import_summary_rejected"
	MsgImportSummaryCatalog   MsgID = "import_summary_catalog"
	MsgImportWhereVariant     MsgID = "import_where_variant"
	MsgImportWhereAnalog      MsgID = "import_where_analog"
	MsgImportWhereValue       MsgID = "import_where_value"
	MsgImportCatalogPlain     MsgID = "import_catalog_object_plain"
	MsgImportNdjsonSubsection MsgID = "import_ndjson_subsection_array"

	// Сообщения хранилища (internal/storage) и сидирования.
	MsgStorageDsnMissing         MsgID = "storage_dsn_missing"
	MsgStorageDialectUnknown     MsgID = "storage_dialect_unknown"
	MsgStorageOpen               MsgID = "storage_open_failed"
	MsgStorageDDL                MsgID = "storage_ddl_failed"
	MsgStorageCatalogWrite       MsgID = "storage_catalog_write_failed"
	MsgStorageRereadMissing      MsgID = "storage_reread_missing"
	MsgStorageRevisionRowMissing MsgID = "storage_revision_row_missing"
	MsgStoragePageInvalid        MsgID = "storage_page_invalid"
	MsgStorageCommitReadOnly     MsgID = "storage_commit_read_only"
	MsgSeedFailed                MsgID = "seed_failed"

	// Сообщения сервисного слоя (internal/service).
	MsgSvcRecordNameMissing     MsgID = "svc_record_name_missing"
	MsgSvcRecordAlreadyExists   MsgID = "svc_record_already_exists"
	MsgSvcSectionUnknown        MsgID = "svc_section_unknown"
	MsgSvcSectionMissing        MsgID = "svc_section_missing"
	MsgSvcSectionDuplicate      MsgID = "svc_section_duplicate"
	MsgSvcAttrCodeMissing       MsgID = "svc_attr_code_missing"
	MsgSvcAttrDuplicate         MsgID = "svc_attr_duplicate"
	MsgSvcManufacturerEmpty     MsgID = "svc_manufacturer_empty"
	MsgSvcManufacturerDuplicate MsgID = "svc_manufacturer_duplicate"
	MsgSvcAnalogEmpty           MsgID = "svc_analog_empty"
	MsgSvcAnalogSelf            MsgID = "svc_analog_self"
	MsgSvcAnalogDuplicate       MsgID = "svc_analog_duplicate"
	MsgSvcAnalogNotFound        MsgID = "svc_analog_not_found"
	MsgSvcFilterFieldMissing    MsgID = "svc_designation_filter_field_missing"
	MsgSvcFilterOpUnknown       MsgID = "svc_filter_op_unknown"
	MsgSvcSearchFieldUnknown    MsgID = "svc_search_field_unknown"
	MsgSvcFilterAttrText        MsgID = "svc_filter_attr_text_expected"
	MsgSvcFilterAttrNumber      MsgID = "svc_filter_attr_number_expected"
	MsgSvcFilterParamSet        MsgID = "svc_filter_param_value_missing"
	MsgSvcFilterParamText       MsgID = "svc_filter_param_text_expected"
	MsgSvcFilterParamNumber     MsgID = "svc_filter_param_number_expected"
	MsgSvcFieldDuplicate        MsgID = "svc_field_duplicate"
	MsgSvcFieldUnknown          MsgID = "svc_field_unknown"
	MsgSvcFieldParserOwned      MsgID = "svc_field_parser_owned"
	MsgSvcFieldBaseDerived      MsgID = "svc_field_base_derived"
	MsgSvcFieldNotApplicable    MsgID = "svc_field_not_applicable"
	MsgSvcFieldAssemblyRange    MsgID = "svc_field_assembly_range"
	MsgSvcFieldTextExpected     MsgID = "svc_field_text_expected"
	MsgSvcFieldDevNumber        MsgID = "svc_field_dev_number"

	// Транспортные сообщения REST (internal/httpapi).
	MsgApiRouteNotFound       MsgID = "api_route_not_found"
	MsgApiMethodNotAllowed    MsgID = "api_method_not_allowed"
	MsgApiPanic               MsgID = "api_panic"
	MsgApiBodyTooLarge        MsgID = "api_body_too_large"
	MsgApiBodyRead            MsgID = "api_body_read"
	MsgApiCardNotFound        MsgID = "api_card_not_found"
	MsgApiIdNotFound          MsgID = "api_id_not_found"
	MsgApiAlreadyExists       MsgID = "api_already_exists"
	MsgApiDesignationMismatch MsgID = "api_designation_mismatch"
	MsgApiBadID               MsgID = "api_bad_id"
	MsgApiQMissing            MsgID = "api_q_missing"
	MsgApiDesignationMissing  MsgID = "api_designation_missing"
	MsgApiQueryInvalid        MsgID = "api_query_invalid"
	MsgApiLimitRange          MsgID = "api_limit_range"
	MsgApiOffsetNonNeg        MsgID = "api_offset_non_negative"
	MsgApiSortKey             MsgID = "api_sort_key_unknown"
	MsgApiQueryParamUnknown   MsgID = "api_query_param_unknown"
	MsgApiParamDuplicate      MsgID = "api_param_duplicate"
	MsgApiNumberParam         MsgID = "api_number_param"
	MsgApiBoolAttrFilter      MsgID = "api_bool_attr_filter"
	MsgInternalError          MsgID = "internal_error"

	// Интерфейсные строки CLI (internal/cli).
	MsgCliUnknownCommand    MsgID = "cli_unknown_command"
	MsgCliOptionsAfterCmd   MsgID = "cli_options_after_command"
	MsgCliDsnDialect        MsgID = "cli_dsn_dialect_mismatch"
	MsgCliArgCount          MsgID = "cli_arg_count"
	MsgCliHelpArgs          MsgID = "cli_help_args"
	MsgCliListArg           MsgID = "cli_list_arg"
	MsgCliListAllFlags      MsgID = "cli_list_all_flags"
	MsgCliUnknownFlag       MsgID = "cli_unknown_flag"
	MsgCliFlagValueRequired MsgID = "cli_flag_value_required"
	MsgCliFlagNoValue       MsgID = "cli_flag_no_value"
	MsgCliLangInvalid       MsgID = "cli_lang_invalid"
	MsgCliFlagNonNegative   MsgID = "cli_flag_non_negative"
	MsgCliFlagNumber        MsgID = "cli_flag_number"
	MsgCliUnknownFlagSet    MsgID = "cli_unknown_flag_set"
	MsgCliDialectUnknown    MsgID = "cli_dialect_unknown"
	MsgCliCatalogSubMissing MsgID = "cli_catalog_subcommand_missing"
	MsgCliCatalogExportArgs MsgID = "cli_catalog_export_args"
	MsgCliCatalogImportArgs MsgID = "cli_catalog_import_args"
	MsgCliCatalogListArgs   MsgID = "cli_catalog_list_args"
	MsgCliCatalogSubUnknown MsgID = "cli_catalog_subcommand_unknown"
	MsgCliFileOpen          MsgID = "cli_file_open"
	MsgCliFileWrite         MsgID = "cli_file_write"
	MsgCliErrPrefix         MsgID = "cli_err_prefix"
	MsgCliUnexpectedPrefix  MsgID = "cli_unexpected_prefix"
	MsgCliDbInitialized     MsgID = "cli_db_initialized"
	MsgCliOutcomeAdded      MsgID = "cli_outcome_added"
	MsgCliOutcomeUpdated    MsgID = "cli_outcome_updated"
	MsgCliOutcomeSkipped    MsgID = "cli_outcome_skipped"
	MsgCliRecordNotFound    MsgID = "cli_record_not_found"
	MsgCliFindSuggestion    MsgID = "cli_find_suggestion"
	MsgCliFindType          MsgID = "cli_find_type"
	MsgCliFindNotFound      MsgID = "cli_find_not_found"
	MsgCliDeleteDryRun      MsgID = "cli_delete_dry_run"
	MsgCliDeleted           MsgID = "cli_deleted"
	MsgCliDryRunHead        MsgID = "cli_dry_run_head"
	MsgCliIssueLine         MsgID = "cli_issue_line"
	MsgCliSystemLine        MsgID = "cli_system_line"
	MsgCliVariantHead       MsgID = "cli_variant_head"
	MsgCliTypeGroupHead     MsgID = "cli_type_group_head"
	MsgCliNoLabel           MsgID = "cli_no_label"
	MsgCliManufacturers     MsgID = "cli_manufacturers"
	MsgCliAnalogs           MsgID = "cli_analogs"
	MsgCliBacklinks         MsgID = "cli_backlinks"
	MsgCliParseHead         MsgID = "cli_parse_head"
	MsgCliFieldLine         MsgID = "cli_field_line"
	MsgCliAtLeast           MsgID = "cli_at_least"
	MsgCliAtMost            MsgID = "cli_at_most"
	MsgCliCondAt            MsgID = "cli_cond_at"
	MsgCliAssemblyValue     MsgID = "cli_assembly_value"
	MsgCliDeviceValue       MsgID = "cli_device_value"
	MsgCliHKinds            MsgID = "cli_h_kinds"
	MsgCliHSystems          MsgID = "cli_h_systems"
	MsgCliHGroups           MsgID = "cli_h_groups"
	MsgCliHParams           MsgID = "cli_h_params"
	MsgCliHAttrs            MsgID = "cli_h_attrs"
	MsgCliGroupLine         MsgID = "cli_group_line"
	MsgCliParamLine         MsgID = "cli_param_line"
	MsgCliAttrListLine      MsgID = "cli_attr_list_line"
	MsgCliUsageWord         MsgID = "cli_usage_word"

	// Фрагменты общей справки (catalogctl help и вызов без аргументов):
	// текст собирается генератором internal/cli/help.go по реестру команд
	// (порядок, колонки, перенос строк), строки — каталог сообщений.
	// Структурные: заголовок, заголовки секций, футер о префиксах ошибок
	// (аргументы — вложенные сообщения префиксов).
	MsgCliUsageHeader   MsgID = "cli_usage_header"
	MsgCliUsageCommands MsgID = "cli_usage_commands"
	MsgCliUsageOptions  MsgID = "cli_usage_options"
	MsgCliUsageFooter   MsgID = "cli_usage_footer"

	// Команды: синопсис и описание (list — с перечнем фильтров реестра).
	MsgCliUsageCmdInit              MsgID = "cli_usage_cmd_init"
	MsgCliUsageCmdInitDesc          MsgID = "cli_usage_cmd_init_desc"
	MsgCliUsageCmdParse             MsgID = "cli_usage_cmd_parse"
	MsgCliUsageCmdParseDesc         MsgID = "cli_usage_cmd_parse_desc"
	MsgCliUsageCmdAdd               MsgID = "cli_usage_cmd_add"
	MsgCliUsageCmdAddDesc           MsgID = "cli_usage_cmd_add_desc"
	MsgCliUsageCmdImport            MsgID = "cli_usage_cmd_import"
	MsgCliUsageCmdImportDesc        MsgID = "cli_usage_cmd_import_desc"
	MsgCliUsageCmdList              MsgID = "cli_usage_cmd_list"
	MsgCliUsageCmdListDesc          MsgID = "cli_usage_cmd_list_desc"
	MsgCliUsageCmdInfo              MsgID = "cli_usage_cmd_info"
	MsgCliUsageCmdInfoDesc          MsgID = "cli_usage_cmd_info_desc"
	MsgCliUsageCmdFind              MsgID = "cli_usage_cmd_find"
	MsgCliUsageCmdFindDesc          MsgID = "cli_usage_cmd_find_desc"
	MsgCliUsageCmdDelete            MsgID = "cli_usage_cmd_delete"
	MsgCliUsageCmdDeleteDesc        MsgID = "cli_usage_cmd_delete_desc"
	MsgCliUsageCmdCount             MsgID = "cli_usage_cmd_count"
	MsgCliUsageCmdCountDesc         MsgID = "cli_usage_cmd_count_desc"
	MsgCliUsageCmdExport            MsgID = "cli_usage_cmd_export"
	MsgCliUsageCmdExportDesc        MsgID = "cli_usage_cmd_export_desc"
	MsgCliUsageCmdCatalogExport     MsgID = "cli_usage_cmd_catalog_export"
	MsgCliUsageCmdCatalogExportDesc MsgID = "cli_usage_cmd_catalog_export_desc"
	MsgCliUsageCmdCatalogImport     MsgID = "cli_usage_cmd_catalog_import"
	MsgCliUsageCmdCatalogImportDesc MsgID = "cli_usage_cmd_catalog_import_desc"
	MsgCliUsageCmdCatalogList       MsgID = "cli_usage_cmd_catalog_list"
	MsgCliUsageCmdCatalogListDesc   MsgID = "cli_usage_cmd_catalog_list_desc"
	MsgCliUsageCmdHelp              MsgID = "cli_usage_cmd_help"
	MsgCliUsageCmdHelpDesc          MsgID = "cli_usage_cmd_help_desc"

	// Общие опции: спецификация и описание.
	MsgCliUsageOptDialect     MsgID = "cli_usage_opt_dialect"
	MsgCliUsageOptDialectDesc MsgID = "cli_usage_opt_dialect_desc"
	MsgCliUsageOptDb          MsgID = "cli_usage_opt_db"
	MsgCliUsageOptDbDesc      MsgID = "cli_usage_opt_db_desc"
	MsgCliUsageOptDsn         MsgID = "cli_usage_opt_dsn"
	MsgCliUsageOptDsnDesc     MsgID = "cli_usage_opt_dsn_desc"
	MsgCliUsageOptKind        MsgID = "cli_usage_opt_kind"
	MsgCliUsageOptKindDesc    MsgID = "cli_usage_opt_kind_desc"
	MsgCliUsageOptSystem      MsgID = "cli_usage_opt_system"
	MsgCliUsageOptSystemDesc  MsgID = "cli_usage_opt_system_desc"
	MsgCliUsageOptLang        MsgID = "cli_usage_opt_lang"
	MsgCliUsageOptLangDesc    MsgID = "cli_usage_opt_lang_desc"
	MsgCliUsageOptDryRun      MsgID = "cli_usage_opt_dry_run"
	MsgCliUsageOptDryRunDesc  MsgID = "cli_usage_opt_dry_run_desc"
	MsgCliUsageOptFormat      MsgID = "cli_usage_opt_format"
	MsgCliUsageOptFormatDesc  MsgID = "cli_usage_opt_format_desc"

	// Строка использования команды (карточка help <команда> и ошибки
	// числа аргументов): путь и позиционные аргументы, %[1]s — полный
	// перечень флагов реестра команды («[--имя] …», генерируется).
	MsgCliUseInit          MsgID = "cli_use_init"
	MsgCliUseParse         MsgID = "cli_use_parse"
	MsgCliUseAdd           MsgID = "cli_use_add"
	MsgCliUseImport        MsgID = "cli_use_import"
	MsgCliUseList          MsgID = "cli_use_list"
	MsgCliUseInfo          MsgID = "cli_use_info"
	MsgCliUseFind          MsgID = "cli_use_find"
	MsgCliUseDelete        MsgID = "cli_use_delete"
	MsgCliUseCount         MsgID = "cli_use_count"
	MsgCliUseExport        MsgID = "cli_use_export"
	MsgCliUseCatalogExport MsgID = "cli_use_catalog_export"
	MsgCliUseCatalogImport MsgID = "cli_use_catalog_import"
	MsgCliUseCatalogList   MsgID = "cli_use_catalog_list"
	MsgCliUseVersion       MsgID = "cli_use_version"
	MsgCliUseHelp          MsgID = "cli_use_help"

	// Карточка команды: заголовок секции опций.
	MsgCliUsageFlags MsgID = "cli_usage_flags"

	// Фильтры list (спецификации и описания опций карточек и общей
	// справки).
	MsgCliUsageOptMaterial       MsgID = "cli_usage_opt_material"
	MsgCliUsageOptMaterialDesc   MsgID = "cli_usage_opt_material_desc"
	MsgCliUsageOptSubclass       MsgID = "cli_usage_opt_subclass"
	MsgCliUsageOptSubclassDesc   MsgID = "cli_usage_opt_subclass_desc"
	MsgCliUsageOptAdjustment     MsgID = "cli_usage_opt_adjustment"
	MsgCliUsageOptAdjustmentDesc MsgID = "cli_usage_opt_adjustment_desc"
	MsgCliUsageOptCategory       MsgID = "cli_usage_opt_category"
	MsgCliUsageOptCategoryDesc   MsgID = "cli_usage_opt_category_desc"
	MsgCliUsageOptJunctions      MsgID = "cli_usage_opt_junctions"
	MsgCliUsageOptJunctionsDesc  MsgID = "cli_usage_opt_junctions_desc"
	MsgCliUsageOptGroup          MsgID = "cli_usage_opt_group"
	MsgCliUsageOptGroupDesc      MsgID = "cli_usage_opt_group_desc"
	MsgCliUsageOptSeries         MsgID = "cli_usage_opt_series"
	MsgCliUsageOptSeriesDesc     MsgID = "cli_usage_opt_series_desc"
	MsgCliUsageOptNumber         MsgID = "cli_usage_opt_number"
	MsgCliUsageOptNumberDesc     MsgID = "cli_usage_opt_number_desc"
	MsgCliUsageOptLetters        MsgID = "cli_usage_opt_letters"
	MsgCliUsageOptLettersDesc    MsgID = "cli_usage_opt_letters_desc"
	MsgCliUsageOptBase           MsgID = "cli_usage_opt_base"
	MsgCliUsageOptBaseDesc       MsgID = "cli_usage_opt_base_desc"
	MsgCliUsageOptQ              MsgID = "cli_usage_opt_q"
	MsgCliUsageOptQDesc          MsgID = "cli_usage_opt_q_desc"
	MsgCliUsageOptLimit          MsgID = "cli_usage_opt_limit"
	MsgCliUsageOptLimitDesc      MsgID = "cli_usage_opt_limit_desc"
	MsgCliUsageOptOffset         MsgID = "cli_usage_opt_offset"
	MsgCliUsageOptOffsetDesc     MsgID = "cli_usage_opt_offset_desc"

	// Примеры и примечание об позиции опций общей справки.
	MsgCliUsageExamples MsgID = "cli_usage_examples"
	MsgCliUsageExInit   MsgID = "cli_usage_ex_init"
	MsgCliUsageExImport MsgID = "cli_usage_ex_import"
	MsgCliUsageExList   MsgID = "cli_usage_ex_list"
	MsgCliUsageExInfo   MsgID = "cli_usage_ex_info"
	MsgCliUsagePosition MsgID = "cli_usage_position"

	// Команда version.
	MsgCliUsageCmdVersion     MsgID = "cli_usage_cmd_version"
	MsgCliUsageCmdVersionDesc MsgID = "cli_usage_cmd_version_desc"
	MsgCliVersion             MsgID = "cli_version"
)

// msgRegistry — реестр MsgID (полнота против каталога — тест).
var msgRegistry = map[MsgID]struct{}{
	MsgSchemaVersionMismatch:  {},
	MsgDatabaseNotInitialized: {},
	MsgKindNotSupported:       {},
	MsgGostSubclassDictSync:   {},
	MsgKindAmbiguous:          {},
	MsgKindAmbiguousMismatch:  {},
	MsgKindRequired:           {},

	MsgEmptyDesignation: {},

	MsgScannerExpected:   {},
	MsgScannerUnexpected: {},
	MsgScannerToken:      {},
	MsgScannerEof:        {},

	MsgKindUnknown:     {},
	MsgSystemUnknown:   {},
	MsgUnknownMaterial: {},
	MsgUnknownSubclass: {},
	MsgUnknownAdjust:   {},
	MsgUnknownCategory: {},
	MsgPowerSuffix:     {},
	MsgDevNumberZeros:  {},
	MsgDevNumberDigits: {},

	MsgCanonicalAlphabetMix: {},
	MsgCanonicalBadRune:     {},
	MsgCanonicalNoLetters:   {},
	MsgKindMismatch:         {},
	MsgSystemMismatch:       {},
	MsgAutodetectFailed:     {},
	MsgCapGroupUnknown:      {},

	MsgExpectGostSubclass:             {},
	MsgExpectGostOptoFunction:         {},
	MsgExpectGostDevNumberFull:        {},
	MsgExpectGostDevNumber:            {},
	MsgExpectGostMaterial:             {},
	MsgExpectDevNumber:                {},
	MsgExpectGostFeatureAndDev:        {},
	MsgExpectGostFeatureDigit:         {},
	MsgExpectGostDevOne:               {},
	MsgExpectGostDevOneDigit:          {},
	MsgExpectGostFeatureDevRun:        {},
	MsgExpectGostModification:         {},
	MsgExpectGostChip:                 {},
	MsgExpectGostMakerCode:            {},
	MsgExpectJedecJunctions:           {},
	MsgExpectJedecLetter:              {},
	MsgExpectJedecNumber:              {},
	MsgExpectJedecNumberNoZero:        {},
	MsgExpectJisJunctions:             {},
	MsgExpectJisS:                     {},
	MsgExpectJisClassLetter:           {},
	MsgExpectJisClassRequired:         {},
	MsgExpectJisNumber:                {},
	MsgExpectJisSuffix:                {},
	MsgExpectJisShortForm:             {},
	MsgExpectJisShortNumber:           {},
	MsgExpectProMaterial:              {},
	MsgExpectProClassLetter:           {},
	MsgExpectProClassTable:            {},
	MsgExpectProNumber:                {},
	MsgExpectProConsumerNumber:        {},
	MsgExpectProAnyNumber:             {},
	MsgExpectProSuffix:                {},
	MsgExpectResFamilyCS:              {},
	MsgExpectResFamilyR:               {},
	MsgExpectHyphen:                   {},
	MsgExpectCapGroup:                 {},
	MsgExpectCapPrefix:                {},
	MsgExpectCapSubclassGroup:         {},
	MsgExpectResMaterialGroup6:        {},
	MsgExpectResMaterialGroup2:        {},
	MsgEngineKindMissing:              {},
	MsgEngineKindNoVariants:           {},
	MsgEngineSystemNotApplicable:      {},
	MsgEngineAttrUnknown:              {},
	MsgEngineAttrInactive:             {},
	MsgEngineAttrNotApplicable:        {},
	MsgEngineAttrTextExpected:         {},
	MsgEngineAttrTextEmpty:            {},
	MsgEngineAttrEnumExpected:         {},
	MsgEngineAttrEnumInvalid:          {},
	MsgEngineAttrBoolExpected:         {},
	MsgEngineAttrIntExpected:          {},
	MsgEngineAttrNumExpected:          {},
	MsgEngineAttrWhole:                {},
	MsgEngineAttrPositive:             {},
	MsgEngineParamUnknown:             {},
	MsgEngineParamInactive:            {},
	MsgEngineParamNotApplicable:       {},
	MsgEngineParamWrongSection:        {},
	MsgEngineParamDuplicate:           {},
	MsgEngineParamTextEmpty:           {},
	MsgValueKeyRequired:               {},
	MsgValueKeyForbidden:              {},
	MsgValueKeysPairRequired:          {},
	MsgValueEnumInvalid:               {},
	MsgValueKeyPositive:               {},
	MsgValueKeyCeiling:                {},
	MsgValueMinMax:                    {},
	MsgCondDuplicate:                  {},
	MsgCondUnknown:                    {},
	MsgCondPositive:                   {},
	MsgCondNotAllowed:                 {},
	MsgCondFixed:                      {},
	MsgCondSetMismatch:                {},
	MsgRuleYearRange:                  {},
	MsgRuleYearOrder:                  {},
	MsgRuleTempPair:                   {},
	MsgRuleCapCylIncomplete:           {},
	MsgRuleCapFormMix:                 {},
	MsgRuleCapRectIncomplete:          {},
	MsgRuleVariantVnomMissing:         {},
	MsgRuleVariantCnomMissing:         {},
	MsgRuleVariantVnomDup:             {},
	MsgRuleVariantLabelDup:            {},
	MsgRuleVariantPnomMissing:         {},
	MsgRuleVariantPnomDup:             {},
	MsgRuleRelayVariantRcoilMissing:   {},
	MsgRuleRelayVariantNominalMissing: {},
	MsgMetaSectionNoCode:              {},
	MsgMetaSectionDupCode:             {},
	MsgMetaRuleUnknownSection:         {},
	MsgMetaParamValueType:             {},
	MsgMetaParamRuleUnknown:           {},
	MsgMetaCondModeUnknown:            {},
	MsgMetaAttrValueType:              {},
	MsgMetaAttrRuleUnknown:            {},
	MsgMetaSystemRefMissing:           {},
	MsgMetaSystemKindMissing:          {},
	MsgMetaCondUnitMissing:            {},
	MsgMetaCondCodeReserved:           {},
	MsgMetaGroupNoSection:             {},
	MsgMetaGroupSortNegative:          {},
	MsgMetaGroupSectionDup:            {},
	MsgMetaGroupSectionReserved:       {},
	MsgMetaRuleUnknown:                {},
	MsgMetaKindRuleKindMissing:        {},
	MsgMetaKindRuleUnknown:            {},
	MsgMetaKindRuleRowMissing:         {},
	MsgMetaKindRuleNotDevice:          {},
	MsgMetaParamGroupMissing:          {},
	MsgMetaParamUnitMissing:           {},
	MsgMetaParamUnitForbidden:         {},
	MsgMetaParamEnumRequired:          {},
	MsgMetaParamEnumOnly:              {},
	MsgMetaParamCeiling:               {},
	MsgMetaParamSortNegative:          {},
	MsgMetaParamKindMissing:           {},
	MsgMetaParamRuleRowMissing:        {},
	MsgMetaParamRuleWrongLevel:        {},
	MsgMetaCondSetEmpty:               {},
	MsgMetaCondMissing:                {},
	MsgMetaCondFixedOptional:          {},
	MsgMetaAttrUnitMissing:            {},
	MsgMetaAttrUnitForbidden:          {},
	MsgMetaAttrEnumRequired:           {},
	MsgMetaAttrEnumOnly:               {},
	MsgMetaAttrSortNegative:           {},
	MsgMetaAttrKindMissing:            {},
	MsgMetaAttrRuleRowMissing:         {},
	MsgMetaAttrRuleWrongLevel:         {},
	MsgImportRootNotObject:            {},
	MsgImportRootUnknownKey:           {},
	MsgImportRootSectionArray:         {},
	MsgImportRecordEmptyName:          {},
	MsgImportRecordBadValue:           {},
	MsgImportRecordNameRequired:       {},
	MsgImportRecordNameString:         {},
	MsgImportRecordUnknownField:       {},
	MsgImportRecordSystemString:       {},
	MsgImportRecordSystemUnknown:      {},
	MsgImportFieldsObject:             {},
	MsgImportFieldUnknown:             {},
	MsgImportFieldString:              {},
	MsgImportFieldNumber:              {},
	MsgImportSectionArray:             {},
	MsgImportManufacturersArray:       {},
	MsgImportManufacturerItem:         {},
	MsgImportVariantsArray:            {},
	MsgImportAnalogsArray:             {},
	MsgImportAttributesObject:         {},
	MsgImportAttrNumber:               {},
	MsgImportAttrBadValue:             {},
	MsgImportWhereSection:             {},
	MsgImportValueObject:              {},
	MsgImportValueParameterString:     {},
	MsgImportValueTextString:          {},
	MsgImportValueKeyNumber:           {},
	MsgImportValueUnknownKey:          {},
	MsgImportValueParameterRequired:   {},
	MsgImportVariantLabelString:       {},
	MsgImportVariantUnknownField:      {},
	MsgImportVariantSectionArray:      {},
	MsgImportAnalogNameRequired:       {},
	MsgImportAnalogNoteString:         {},
	MsgImportAnalogUnknownField:       {},
	MsgImportAnalogItem:               {},
	MsgImportNdjsonLineObject:         {},
	MsgImportNdjsonCatalogOrder:       {},
	MsgImportNdjsonClassKey:           {},
	MsgImportNdjsonWrapperSingle:      {},
	MsgImportCatalogObject:            {},
	MsgImportCatalogSubsection:        {},
	MsgImportCatalogRowsArray:         {},
	MsgImportCatalogRowObject:         {},
	MsgImportCatalogUnknownField:      {},
	MsgImportCatalogFieldRequired:     {},
	MsgImportCatalogFieldString:       {},
	MsgImportCatalogFieldNumber:       {},
	MsgImportCatalogFieldInteger:      {},
	MsgImportCatalogFieldBool:         {},
	MsgImportCatalogFieldStrings:      {},
	MsgImportCatalogItemString:        {},
	MsgImportCatalogParamActiveBool:   {},
	MsgImportCatalogAttrActiveBool:    {},
	MsgImportCatalogCondsetsArray:     {},
	MsgImportCatalogCondsetObject:     {},
	MsgImportCatalogCondsetItems:      {},
	MsgImportCatalogCondsetItemObject: {},
	MsgImportCatalogCondsetEmpty:      {},
	MsgImportCatalogRow:               {},
	MsgImportCatalogRowCode:           {},
	MsgImportFormatUnknown:            {},
	MsgImportFormatByExt:              {},
	MsgImportFormatLineOnly:           {},
	MsgImportYamlNonFinite:            {},
	MsgImportYamlMultiDoc:             {},
	MsgImportYamlAliasCycle:           {},
	MsgImportYamlAliasUnknown:         {},
	MsgImportYamlKeyInvalid:           {},
	MsgImportYamlNodeUnsupported:      {},
	MsgImportNdjsonLineError:          {},
	MsgImportSyntaxError:              {},
	MsgImportCatalogRecordsMixed:      {},
	MsgImportDuplicateKey:             {},
	MsgImportUnexpectedToken:          {},
	MsgImportIssueRecordLine:          {},
	MsgImportIssueRecord:              {},
	MsgImportIssueLine:                {},
	MsgImportIssueNo:                  {},
	MsgImportValueKindNumber:          {},
	MsgImportValueKindString:          {},
	MsgImportValueKindArray:           {},
	MsgImportValueKindObject:          {},
	MsgImportValueKindBool:            {},
	MsgImportValueKindOther:           {},
	MsgImportSummary:                  {},
	MsgImportSummaryRejected:          {},
	MsgImportSummaryCatalog:           {},
	MsgImportWhereVariant:             {},
	MsgImportWhereAnalog:              {},
	MsgImportWhereValue:               {},
	MsgImportCatalogPlain:             {},
	MsgImportNdjsonSubsection:         {},
	MsgStorageDsnMissing:              {},
	MsgStorageDialectUnknown:          {},
	MsgStorageOpen:                    {},
	MsgStorageDDL:                     {},
	MsgStorageCatalogWrite:            {},
	MsgStorageRereadMissing:           {},
	MsgStorageRevisionRowMissing:      {},
	MsgStoragePageInvalid:             {},
	MsgStorageCommitReadOnly:          {},
	MsgSeedFailed:                     {},
	MsgSvcRecordNameMissing:           {},
	MsgSvcRecordAlreadyExists:         {},
	MsgSvcSectionUnknown:              {},
	MsgSvcSectionMissing:              {},
	MsgSvcSectionDuplicate:            {},
	MsgSvcAttrCodeMissing:             {},
	MsgSvcAttrDuplicate:               {},
	MsgSvcManufacturerEmpty:           {},
	MsgSvcManufacturerDuplicate:       {},
	MsgSvcAnalogEmpty:                 {},
	MsgSvcAnalogSelf:                  {},
	MsgSvcAnalogDuplicate:             {},
	MsgSvcAnalogNotFound:              {},
	MsgSvcFilterFieldMissing:          {},
	MsgSvcFilterOpUnknown:             {},
	MsgSvcFieldUnknown:                {},
	MsgSvcFilterAttrText:              {},
	MsgSvcFilterAttrNumber:            {},
	MsgSvcFilterParamSet:              {},
	MsgSvcFilterParamText:             {},
	MsgSvcFilterParamNumber:           {},
	MsgSvcFieldDuplicate:              {},
	MsgSvcSearchFieldUnknown:          {},
	MsgSvcFieldParserOwned:            {},
	MsgSvcFieldBaseDerived:            {},
	MsgSvcFieldNotApplicable:          {},
	MsgSvcFieldAssemblyRange:          {},
	MsgSvcFieldTextExpected:           {},
	MsgSvcFieldDevNumber:              {},
	MsgApiRouteNotFound:               {},
	MsgApiMethodNotAllowed:            {},
	MsgApiPanic:                       {},
	MsgApiBodyTooLarge:                {},
	MsgApiBodyRead:                    {},
	MsgApiCardNotFound:                {},
	MsgApiIdNotFound:                  {},
	MsgApiAlreadyExists:               {},
	MsgApiDesignationMismatch:         {},
	MsgApiBadID:                       {},
	MsgApiQueryInvalid:                {},
	MsgApiQMissing:                    {},
	MsgApiDesignationMissing:          {},
	MsgApiLimitRange:                  {},
	MsgApiOffsetNonNeg:                {},
	MsgApiSortKey:                     {},
	MsgApiQueryParamUnknown:           {},
	MsgApiParamDuplicate:              {},
	MsgApiNumberParam:                 {},
	MsgApiBoolAttrFilter:              {},
	MsgInternalError:                  {},
	MsgCliUnknownCommand:              {},
	MsgCliOptionsAfterCmd:             {},
	MsgCliDsnDialect:                  {},
	MsgCliArgCount:                    {},
	MsgCliHelpArgs:                    {},
	MsgCliListArg:                     {},
	MsgCliListAllFlags:                {},
	MsgCliUnknownFlag:                 {},
	MsgCliFlagValueRequired:           {},
	MsgCliFlagNoValue:                 {},
	MsgCliLangInvalid:                 {},
	MsgCliFlagNonNegative:             {},
	MsgCliFlagNumber:                  {},
	MsgCliUnknownFlagSet:              {},
	MsgCliDialectUnknown:              {},
	MsgCliCatalogSubMissing:           {},
	MsgCliCatalogExportArgs:           {},
	MsgCliCatalogImportArgs:           {},
	MsgCliCatalogListArgs:             {},
	MsgCliCatalogSubUnknown:           {},
	MsgCliFileOpen:                    {},
	MsgCliFileWrite:                   {},
	MsgCliErrPrefix:                   {},
	MsgCliUnexpectedPrefix:            {},
	MsgCliDbInitialized:               {},
	MsgCliOutcomeAdded:                {},
	MsgCliOutcomeUpdated:              {},
	MsgCliOutcomeSkipped:              {},
	MsgCliRecordNotFound:              {},
	MsgCliFindSuggestion:              {},
	MsgCliFindType:                    {},
	MsgCliFindNotFound:                {},
	MsgCliDeleteDryRun:                {},
	MsgCliDeleted:                     {},
	MsgCliDryRunHead:                  {},
	MsgCliIssueLine:                   {},
	MsgCliSystemLine:                  {},
	MsgCliVariantHead:                 {},
	MsgCliTypeGroupHead:               {},
	MsgCliNoLabel:                     {},
	MsgCliManufacturers:               {},
	MsgCliAnalogs:                     {},
	MsgCliBacklinks:                   {},
	MsgCliParseHead:                   {},
	MsgCliFieldLine:                   {},
	MsgCliAtLeast:                     {},
	MsgCliAtMost:                      {},
	MsgCliCondAt:                      {},
	MsgCliAssemblyValue:               {},
	MsgCliDeviceValue:                 {},
	MsgCliHKinds:                      {},
	MsgCliHSystems:                    {},
	MsgCliHGroups:                     {},
	MsgCliHParams:                     {},
	MsgCliHAttrs:                      {},
	MsgCliGroupLine:                   {},
	MsgCliParamLine:                   {},
	MsgCliAttrListLine:                {},
	MsgCliUsageWord:                   {},
	MsgCliUsageHeader:                 {},
	MsgCliUsageCommands:               {},
	MsgCliUsageOptions:                {},
	MsgCliUsageFooter:                 {},
	MsgCliUsageCmdInit:                {},
	MsgCliUsageCmdInitDesc:            {},
	MsgCliUsageCmdParse:               {},
	MsgCliUsageCmdParseDesc:           {},
	MsgCliUsageCmdAdd:                 {},
	MsgCliUsageCmdAddDesc:             {},
	MsgCliUsageCmdImport:              {},
	MsgCliUsageCmdImportDesc:          {},
	MsgCliUsageCmdList:                {},
	MsgCliUsageCmdListDesc:            {},
	MsgCliUsageCmdInfo:                {},
	MsgCliUsageCmdInfoDesc:            {},
	MsgCliUsageCmdFind:                {},
	MsgCliUsageCmdFindDesc:            {},
	MsgCliUsageCmdDelete:              {},
	MsgCliUsageCmdDeleteDesc:          {},
	MsgCliUsageCmdCount:               {},
	MsgCliUsageCmdCountDesc:           {},
	MsgCliUsageCmdExport:              {},
	MsgCliUsageCmdExportDesc:          {},
	MsgCliUsageCmdCatalogExport:       {},
	MsgCliUsageCmdCatalogExportDesc:   {},
	MsgCliUsageCmdCatalogImport:       {},
	MsgCliUsageCmdCatalogImportDesc:   {},
	MsgCliUsageCmdCatalogList:         {},
	MsgCliUsageCmdCatalogListDesc:     {},
	MsgCliUsageCmdHelp:                {},
	MsgCliUsageCmdHelpDesc:            {},
	MsgCliUsageOptDialect:             {},
	MsgCliUsageOptDialectDesc:         {},
	MsgCliUsageOptDb:                  {},
	MsgCliUsageOptDbDesc:              {},
	MsgCliUsageOptDsn:                 {},
	MsgCliUsageOptDsnDesc:             {},
	MsgCliUsageOptKind:                {},
	MsgCliUsageOptKindDesc:            {},
	MsgCliUsageOptSystem:              {},
	MsgCliUsageOptSystemDesc:          {},
	MsgCliUsageOptLang:                {},
	MsgCliUsageOptLangDesc:            {},
	MsgCliUsageOptDryRun:              {},
	MsgCliUsageOptDryRunDesc:          {},
	MsgCliUsageOptFormat:              {},
	MsgCliUsageOptFormatDesc:          {},
	MsgCliUseInit:                     {},
	MsgCliUseParse:                    {},
	MsgCliUseAdd:                      {},
	MsgCliUseImport:                   {},
	MsgCliUseList:                     {},
	MsgCliUseInfo:                     {},
	MsgCliUseFind:                     {},
	MsgCliUseDelete:                   {},
	MsgCliUseCount:                    {},
	MsgCliUseExport:                   {},
	MsgCliUseCatalogExport:            {},
	MsgCliUseCatalogImport:            {},
	MsgCliUseCatalogList:              {},
	MsgCliUseVersion:                  {},
	MsgCliUseHelp:                     {},
	MsgCliUsageFlags:                  {},
	MsgCliUsageOptMaterial:            {},
	MsgCliUsageOptMaterialDesc:        {},
	MsgCliUsageOptSubclass:            {},
	MsgCliUsageOptSubclassDesc:        {},
	MsgCliUsageOptAdjustment:          {},
	MsgCliUsageOptAdjustmentDesc:      {},
	MsgCliUsageOptCategory:            {},
	MsgCliUsageOptCategoryDesc:        {},
	MsgCliUsageOptJunctions:           {},
	MsgCliUsageOptJunctionsDesc:       {},
	MsgCliUsageOptGroup:               {},
	MsgCliUsageOptGroupDesc:           {},
	MsgCliUsageOptSeries:              {},
	MsgCliUsageOptSeriesDesc:          {},
	MsgCliUsageOptNumber:              {},
	MsgCliUsageOptNumberDesc:          {},
	MsgCliUsageOptLetters:             {},
	MsgCliUsageOptLettersDesc:         {},
	MsgCliUsageOptBase:                {},
	MsgCliUsageOptBaseDesc:            {},
	MsgCliUsageOptQ:                   {},
	MsgCliUsageOptQDesc:               {},
	MsgCliUsageOptLimit:               {},
	MsgCliUsageOptLimitDesc:           {},
	MsgCliUsageOptOffset:              {},
	MsgCliUsageOptOffsetDesc:          {},
	MsgCliUsageExamples:               {},
	MsgCliUsageExInit:                 {},
	MsgCliUsageExImport:               {},
	MsgCliUsageExList:                 {},
	MsgCliUsageExInfo:                 {},
	MsgCliUsagePosition:               {},
	MsgCliUsageCmdVersion:             {},
	MsgCliUsageCmdVersionDesc:         {},
	MsgCliVersion:                     {},
}

// MsgIDs — все MsgID реестра (для теста полноты каталога).
func MsgIDs() []MsgID {
	return slices.Collect(maps.Keys(msgRegistry))
}

// Msgf возвращает канонический (en) текст сообщения — для заполнения
// Message при конструировании ошибок.
func Msgf(id MsgID, args ...any) string {
	return i18n.Canonical(string(id), args...)
}

// MsgArg возвращает аргумент-сообщение для слоёв над domain: при рендере
// родительского формата подставляется локализованный текст вложенного
// сообщения (позиционные префиксы вида «раздел units, «кВ»»).
func MsgArg(id MsgID, args ...any) any {
	return i18n.Arg(string(id), args...)
}

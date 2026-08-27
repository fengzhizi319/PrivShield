/**
 * Canonical Data Source IDs and API Codes (Single Source of Truth).
 * 与 Go pkg/naming 和 Python engine/naming 保持 100% 对齐。
 */

export const DS_YIBAO = 'ds_yibao';
export const DS_KANGYANG = 'ds_kangyang';
export const DS_MOCK3 = 'ds_mock3';
export const DS_MOCK4 = 'ds_mock4';

export const API1_YIBAO = 'api1_yibao';
export const API2_KANGYANG = 'api2_kangyang';

export interface CatalogEntry {
  api_code?: string;
  datasource_id: string;
  seq: number;
  display_name: {
    'zh-CN': string;
    'en-US': string;
  };
  category: string;
  file_name: string;
  field_count: number;
  status: 'active' | 'reserved';
}

export const CATALOG: CatalogEntry[] = [
  {
    api_code: API1_YIBAO,
    datasource_id: DS_YIBAO,
    seq: 1,
    display_name: {
      'zh-CN': '医保结算数据接口',
      'en-US': 'Medical Insurance Settlement API',
    },
    category: 'medical',
    file_name: 'yibao.csv',
    field_count: 18,
    status: 'active',
  },
  {
    api_code: API2_KANGYANG,
    datasource_id: DS_KANGYANG,
    seq: 2,
    display_name: {
      'zh-CN': '康养健康档案接口',
      'en-US': 'Elderly-Care Health Record API',
    },
    category: 'healthcare',
    file_name: 'kangyang.csv',
    field_count: 27,
    status: 'active',
  },
  {
    datasource_id: DS_MOCK3,
    seq: 3,
    display_name: {
      'zh-CN': '预留政务数据源 3',
      'en-US': 'Reserved Municipal Dataset 3',
    },
    category: 'reserved',
    file_name: 'mock3.csv',
    field_count: 0,
    status: 'reserved',
  },
  {
    datasource_id: DS_MOCK4,
    seq: 4,
    display_name: {
      'zh-CN': '预留企业/金融数据源 4',
      'en-US': 'Reserved Enterprise Dataset 4',
    },
    category: 'reserved',
    file_name: 'mock4.csv',
    field_count: 0,
    status: 'reserved',
  },
];

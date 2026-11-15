export interface StorageProduct {
  id: string;
  name: string;
  size: number;
  time: number;
  price: number;
  points?: number;
  chip?: string;
}

export interface GroupProduct {
  id: string;
  name: string;
  group_id: number;
  time: number;
  price: number;
  points?: number;
  chip?: string;
  des?: string[];
}

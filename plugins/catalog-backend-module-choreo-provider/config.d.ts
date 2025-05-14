export interface Config {
  choreoIngestor: {
    enabled: boolean;
    taskRunner: {
      frequency: number;
      timeout: number;
    };
    excludedNamespaces: string[];
  };
}

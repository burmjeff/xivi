extern crate libc;
extern crate alloc;

use candle_transformers::models::bert::{Config, BertModel, DTYPE};
use candle_core::{Device, NdArray, Tensor};
use candle_nn::VarBuilder;
use hf_hub::{api::sync::Api, Repo, RepoType};
use tokenizers::Tokenizer;
use anyhow::{Error as E, Result};
use core::ffi::{CStr, c_char};
use alloc::{ffi::CString, vec::Vec, string::ToString};

#[derive()]
pub struct EmbeddingsModel {
    model: BertModel,
    tokenizer: Tokenizer,
}

impl EmbeddingsModel {
    pub fn new() -> Result<Self> {
        let device = Device::Cpu;
        let model_id = "avsolatorio/GIST-small-Embedding-v0".to_string();
        let revision = "main".to_string();

        let repo = Repo::with_revision(model_id.clone(), RepoType::Model, revision.clone());
        let (config_filename, tokenizer_filename, weights_filename) = {
            let api = Api::new()?;
            let api = api.repo(repo);
            let config = api.get("config.json")?;
            let tokenizer = api.get("tokenizer.json")?;
            let weights = api.get("model.safetensors")?;
            (config, tokenizer, weights)
        };
        let config_data = std::fs::read_to_string(config_filename)?;
        let config: Config = serde_json::from_str(&config_data)?;
        let tokenizer = Tokenizer::from_file(tokenizer_filename).map_err(E::msg)?;

        let vb = unsafe { VarBuilder::from_mmaped_safetensors(&[weights_filename], DTYPE, &device)? };
        let model = BertModel::load(vb, &config)?;

        Ok(Self {
            model,
            tokenizer,
        })
    }


    pub fn get_embedding(&mut self, prompt: &str) -> Result<CString> {
        let tokenizer = &self.tokenizer
            .with_padding(None)
            .with_truncation(None)
            .map_err(E::msg)?;
        let tokens = tokenizer
            .encode(prompt, true)
            .map_err(E::msg)?
            .get_ids()
            .to_vec();
        let token_ids = Tensor::new(&tokens[..], &self.model.device)?.unsqueeze(0)?;
        let token_type_ids = token_ids.zeros_like()?;

        let embedding = self.model.forward(&token_ids, &token_type_ids)?;
        let (_n_sentence, n_tokens, _hidden_size) = embedding.dims3()?;
        let embedding = (embedding.sum(1)? / (n_tokens as f64))?;
        let embedding = self.normalize_l2(&embedding)?;
        let tensor_vec = embedding.get(0)?.to_vec1::<f32>()?;

        let json_data = serde_json::to_vec(&tensor_vec).unwrap();
        let c_str = CString::new(json_data).unwrap();
        Ok(c_str)
    }

    fn normalize_l2(&self, v: &Tensor) -> Result<Tensor> {
        Ok(v.broadcast_div(&v.sqr()?.sum_keepdim(1)?.sqrt()?)?)
    }

    fn _get_mask(&self, size: usize) -> Tensor {
        let mask: Vec<_> = (0..size)
            .flat_map(|i| (0..size).map(move |j| u8::from(j > i)))
            .collect();
        Tensor::from_slice(&mask, (size, size), &self.model.device).unwrap()
    }

    fn _get_cosine_similarity(&self, embed1: CString, embed2: CString) -> f32 {
        let tensor_vec1 = serde_json::from_slice::<Vec<f32>>(&embed1.as_bytes()).unwrap();
        let tensor_vec1_clone = tensor_vec1.clone();
        let tensor1 = Tensor::from_vec(tensor_vec1, tensor_vec1_clone.shape().unwrap(),&Device::Cpu).unwrap();
        
        let tensor_vec2 = serde_json::from_slice::<Vec<f32>>(&embed2.as_bytes()).unwrap();
        let tensor_vec2_clone = tensor_vec2.clone();
        let tensor2 = Tensor::from_vec(tensor_vec2, tensor_vec2_clone.shape().unwrap(),&Device::Cpu).unwrap();

        let sum_ij = (&tensor1 * &tensor2).unwrap().sum_all().unwrap().to_scalar::<f32>().unwrap();
        let sum_i2 = (&tensor1 * &tensor1).unwrap().sum_all().unwrap().to_scalar::<f32>().unwrap();
        let sum_j2 = (&tensor2 * &tensor2).unwrap().sum_all().unwrap().to_scalar::<f32>().unwrap();
        return sum_ij / (sum_i2 * sum_j2).sqrt();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_get_embedding() {
        let mut model = EmbeddingsModel::new().unwrap();
        let embedding1 = model.get_embedding("This is a test.").unwrap();
        //assert!(embedding.is_empty());
        let embedding2 = model.get_embedding("This is not a test.").unwrap();
        println!("SIMILARITY: {:?}", model._get_cosine_similarity(embedding1, embedding2));
    }
}

#[no_mangle]
pub unsafe extern "C" fn create_embedding(input: *const c_char) -> *mut libc::c_char {
    let mut model = EmbeddingsModel::new().unwrap();
    let result = model.get_embedding(CStr::from_ptr(input).to_str().unwrap());
    match result {
        Ok(value) => value.into_raw(),
        Err(_) => CString::new("").unwrap().into_raw(), // Handle error appropriately
    }
}